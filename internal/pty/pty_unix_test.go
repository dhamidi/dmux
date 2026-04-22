//go:build unix

package pty

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// readAllWithDeadline drains r until EOF or deadline. Returns
// whatever was collected. pty master reads often return EIO on
// Darwin after the child exits; that is treated the same as EOF.
func readAllWithDeadline(t *testing.T, r io.Reader, d time.Duration) []byte {
	t.Helper()
	done := make(chan struct{})
	var buf bytes.Buffer
	var readErr error
	go func() {
		defer close(done)
		b := make([]byte, 4096)
		for {
			n, err := r.Read(b)
			if n > 0 {
				buf.Write(b[:n])
			}
			if err != nil {
				readErr = err
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("read deadline exceeded; got so far: %q (last err: %v)", buf.String(), readErr)
	}
	return buf.Bytes()
}

func TestSpawnProducesOutput(t *testing.T) {
	p, err := Spawn(context.Background(), Config{
		Argv: []string{"/bin/sh", "-c", "printf foobar"},
		Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()

	out := readAllWithDeadline(t, p, 3*time.Second)
	if !bytes.Contains(out, []byte("foobar")) {
		t.Fatalf("expected output to contain %q, got %q", "foobar", out)
	}
	if _, err := p.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}

func TestExitCode(t *testing.T) {
	p, err := Spawn(context.Background(), Config{
		Argv: []string{"/bin/sh", "-c", "exit 42"},
		Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()

	// Drain so Wait doesn't block on pipe buffer.
	go io.Copy(io.Discard, p)

	st, err := p.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !st.Exited {
		t.Fatalf("expected exited, got signal %d", st.Signal)
	}
	if st.Code != 42 {
		t.Fatalf("expected exit code 42, got %d", st.Code)
	}
}

func TestInitialSize(t *testing.T) {
	p, err := Spawn(context.Background(), Config{
		Argv: []string{"/bin/sh", "-c", "stty size"},
		Cols: 100, Rows: 30,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()

	out := readAllWithDeadline(t, p, 3*time.Second)
	if _, err := p.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	// stty prints "rows cols".
	if !bytes.Contains(out, []byte("30 100")) {
		t.Fatalf("expected output to contain %q, got %q", "30 100", out)
	}
}

func TestResize(t *testing.T) {
	p, err := Spawn(context.Background(), Config{
		// Sleep briefly to let Resize land before stty runs. Using
		// a shell read from stdin would be ideal, but that adds
		// echo-handling noise; a short sleep is enough to observe
		// the resize.
		Argv: []string{"/bin/sh", "-c", "sleep 0.2; stty size"},
		Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()

	if err := p.Resize(120, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	out := readAllWithDeadline(t, p, 3*time.Second)
	if _, err := p.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !bytes.Contains(out, []byte("40 120")) {
		t.Fatalf("expected output to contain %q after resize, got %q", "40 120", out)
	}
}

func TestCloseUnblocksRead(t *testing.T) {
	p, err := Spawn(context.Background(), Config{
		Argv: []string{"/bin/cat"},
		Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 64)
		_, err := p.Read(buf)
		done <- err
	}()

	// Give the reader a moment to park in Read.
	time.Sleep(50 * time.Millisecond)
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected Read to return error after Close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Read did not unblock after Close")
	}

	// A subsequent Read reports ErrClosed via the fast path.
	if _, err := p.Read(make([]byte, 1)); !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed on post-close Read, got %v", err)
	}
	// cat is now an orphan but setsid means the kernel reaps it
	// eventually; don't block the test on it.
	go p.Wait()
}

func TestSignalKill(t *testing.T) {
	p, err := Spawn(context.Background(), Config{
		Argv: []string{"/bin/sh", "-c", "while :; do sleep 1; done"},
		Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()
	go io.Copy(io.Discard, p)

	if err := p.Signal(SIGKILL); err != nil {
		t.Fatalf("Signal: %v", err)
	}

	st, err := p.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if st.Exited {
		t.Fatalf("expected signal-terminated, got exit code %d", st.Code)
	}
	if st.Signal != SIGKILL {
		t.Fatalf("expected SIGKILL, got %d", st.Signal)
	}
}

func TestContextCancelCloses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p, err := Spawn(ctx, Config{
		Argv: []string{"/bin/cat"},
		Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 64)
		_, err := p.Read(buf)
		done <- err
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Read did not unblock after ctx cancel")
	}
	go p.Wait()
}

func TestEmptyArgv(t *testing.T) {
	_, err := Spawn(context.Background(), Config{Argv: nil})
	if err == nil {
		t.Fatal("expected error for empty Argv")
	}
	if !errors.Is(err, ErrStartProcess) {
		t.Fatalf("expected ErrStartProcess, got %v", err)
	}
	var se *SpawnError
	if !errors.As(err, &se) {
		t.Fatalf("expected *SpawnError, got %T", err)
	}
	if se.Op != OpStart {
		t.Fatalf("expected Op=%s, got %s", OpStart, se.Op)
	}
}

// Ensure our error wrapping preserves errors.Is all the way down.
func TestOpenPtyErrorPath(t *testing.T) {
	// Sanity: a non-existent argv0 produces ErrStartProcess.
	_, err := Spawn(context.Background(), Config{
		Argv: []string{"/definitely/not/a/real/path/xyzzy"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrStartProcess) {
		t.Fatalf("expected ErrStartProcess, got %v", err)
	}
	if !strings.Contains(err.Error(), "pty:") {
		t.Fatalf("error should be pty-prefixed: %q", err.Error())
	}
}
