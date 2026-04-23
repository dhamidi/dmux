//go:build unix

package pane

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/pty"
	"github.com/dhamidi/dmux/internal/vt"
)

// drainUntilClosed collects every chunk from the pane's Bytes()
// channel until the channel closes or the deadline expires. Used by
// tests that want to see the child's full output after it exits.
func drainUntilClosed(t *testing.T, p *Pane, d time.Duration) []byte {
	t.Helper()
	var buf bytes.Buffer
	deadline := time.After(d)
	for {
		select {
		case chunk, ok := <-p.Bytes():
			if !ok {
				return buf.Bytes()
			}
			buf.Write(chunk)
		case <-deadline:
			t.Fatalf("drain deadline exceeded; got so far: %q", buf.String())
			return buf.Bytes()
		}
	}
}

// drainUntilContains collects from Bytes() until the accumulated
// output contains needle, or the deadline expires. Used for
// interactive tests (e.g. /bin/cat) where the child does not exit.
func drainUntilContains(t *testing.T, p *Pane, needle []byte, d time.Duration) []byte {
	t.Helper()
	var buf bytes.Buffer
	deadline := time.After(d)
	for {
		if bytes.Contains(buf.Bytes(), needle) {
			return buf.Bytes()
		}
		select {
		case chunk, ok := <-p.Bytes():
			if !ok {
				if bytes.Contains(buf.Bytes(), needle) {
					return buf.Bytes()
				}
				t.Fatalf("Bytes channel closed before %q appeared; got %q", needle, buf.String())
				return buf.Bytes()
			}
			buf.Write(chunk)
		case <-deadline:
			t.Fatalf("deadline waiting for %q; got so far: %q", needle, buf.String())
			return buf.Bytes()
		}
	}
}

// waitExited blocks until the Exited() channel yields (and closes)
// or the deadline expires. Returns the received status.
func waitExited(t *testing.T, p *Pane, d time.Duration) pty.ExitStatus {
	t.Helper()
	select {
	case st := <-p.Exited():
		return st
	case <-time.After(d):
		t.Fatal("Exited did not fire before deadline")
		return pty.ExitStatus{}
	}
}

func TestOpenAndRead(t *testing.T) {
	p, err := Open(context.Background(), Config{
		Argv: []string{"/bin/sh", "-c", "printf hello"},
		Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer p.Close()

	out := drainUntilClosed(t, p, 3*time.Second)
	if !bytes.Contains(out, []byte("hello")) {
		t.Fatalf("expected output to contain %q, got %q", "hello", out)
	}

	st := waitExited(t, p, 3*time.Second)
	if !st.Exited {
		t.Fatalf("expected exited, got signal %d", st.Signal)
	}
	if st.Code != 0 {
		t.Fatalf("expected exit code 0, got %d", st.Code)
	}
}

func TestWriteEchoesBack(t *testing.T) {
	// Disable echo so we see ping exactly once (otherwise the pty
	// echoes the input back before cat does). Simpler to assert on.
	p, err := Open(context.Background(), Config{
		Argv: []string{"/bin/sh", "-c", "stty -echo; exec cat"},
		Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer p.Close()

	// Give stty a moment to land before writing. Without this, the
	// shell may still be setting up and echo is briefly on.
	time.Sleep(100 * time.Millisecond)

	if _, err := p.Write([]byte("ping\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	drainUntilContains(t, p, []byte("ping"), 3*time.Second)

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Exited must fire after Close (pty shut down).
	select {
	case <-p.Exited():
	case <-time.After(3 * time.Second):
		t.Fatal("Exited did not fire after Close")
	}
}

func TestResize(t *testing.T) {
	p, err := Open(context.Background(), Config{
		// Sleep to let Resize land before stty runs.
		Argv: []string{"/bin/sh", "-c", "sleep 0.2; stty size; exit"},
		Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer p.Close()

	if err := p.Resize(120, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	out := drainUntilClosed(t, p, 3*time.Second)
	// stty size prints "rows cols".
	if !bytes.Contains(out, []byte("40 120")) {
		t.Fatalf("expected output to contain %q, got %q", "40 120", out)
	}

	waitExited(t, p, 3*time.Second)
}

func TestCloseTerminatesChild(t *testing.T) {
	p, err := Open(context.Background(), Config{
		Argv: []string{"/bin/sh", "-c", "while :; do sleep 1; done"},
		Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Drain in the background so the reader goroutine can progress.
	go func() {
		for range p.Bytes() {
		}
	}()

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Just assert the channel fires within the deadline. Payload
	// contents are platform-dependent (SIGHUP vs. SIGTERM vs. a
	// shell that traps and exits cleanly).
	select {
	case <-p.Exited():
	case <-time.After(2 * time.Second):
		t.Fatal("Exited did not fire within 2s after Close")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	p, err := Open(context.Background(), Config{
		Argv: []string{"/bin/sh", "-c", "printf hi"},
		Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Drain so the reader can finish.
	go func() {
		for range p.Bytes() {
		}
	}()

	if err := p.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second Close should be nil, got %v", err)
	}
}

func TestOpenEmptyArgv(t *testing.T) {
	_, err := Open(context.Background(), Config{Argv: nil})
	if err == nil {
		t.Fatal("expected error for empty Argv")
	}
	// The pty sentinel must still be reachable through the pane's
	// wrapper so callers can dispatch on the spawn failure category.
	if !errors.Is(err, pty.ErrStartProcess) {
		t.Fatalf("expected errors.Is(err, pty.ErrStartProcess), got %v", err)
	}
	// And our own sentinel.
	if !errors.Is(err, ErrSpawn) {
		t.Fatalf("expected errors.Is(err, ErrSpawn), got %v", err)
	}
	var pe *PaneError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *PaneError, got %T", err)
	}
	if pe.Op != OpOpen {
		t.Fatalf("expected Op=%s, got %s", OpOpen, pe.Op)
	}
}

func TestSnapshotFromFeed(t *testing.T) {
	ctx := context.Background()
	rt, err := vt.NewRuntime(ctx)
	if err != nil {
		t.Fatalf("vt.NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })

	p, err := Open(ctx, Config{
		Argv: []string{"/bin/sh", "-c", "printf hello"},
		Cols: 40, Rows: 5,
		VT: rt,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer p.Close()

	// Drain bytesCh to close (implying readLoop finished all Feeds)
	// before snapshotting.
	drainUntilClosed(t, p, 3*time.Second)
	waitExited(t, p, 3*time.Second)

	g, err := p.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if g.Rows < 1 || g.Cols < 5 {
		t.Fatalf("grid too small: rows=%d cols=%d", g.Rows, g.Cols)
	}
	var line strings.Builder
	for _, c := range g.Cells[0] {
		if c.Rune == 0 {
			break
		}
		line.WriteRune(c.Rune)
	}
	if !strings.HasPrefix(line.String(), "hello") {
		t.Errorf("first row = %q, want prefix %q", line.String(), "hello")
	}

	cur, err := p.Cursor()
	if err != nil {
		t.Fatalf("Cursor: %v", err)
	}
	if cur.X != 5 || cur.Y != 0 {
		t.Errorf("cursor = (%d,%d), want (5,0)", cur.X, cur.Y)
	}
}

func TestSnapshotNoVTReturnsErrNoVT(t *testing.T) {
	p, err := Open(context.Background(), Config{
		Argv: []string{"/bin/sh", "-c", "printf hi"},
		Cols: 40, Rows: 5,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer p.Close()

	if _, err := p.Snapshot(); !errors.Is(err, ErrNoVT) {
		t.Errorf("Snapshot without VT: expected ErrNoVT, got %v", err)
	}
	if _, err := p.Cursor(); !errors.Is(err, ErrNoVT) {
		t.Errorf("Cursor without VT: expected ErrNoVT, got %v", err)
	}
}

func TestSubscribeFiresOnFeed(t *testing.T) {
	ctx := context.Background()
	rt, err := vt.NewRuntime(ctx)
	if err != nil {
		t.Fatalf("vt.NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })

	p, err := Open(ctx, Config{
		Argv: []string{"/bin/sh", "-c", "printf hello"},
		Cols: 40, Rows: 5,
		VT:   rt,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer p.Close()

	sub := p.Subscribe()
	defer sub.Close()

	// Subscribe primes the channel once so the caller can do an
	// initial render without waiting for the first feed. Drain that.
	select {
	case <-sub.Ch:
	case <-time.After(time.Second):
		t.Fatal("Subscribe did not prime the channel")
	}

	// Drain bytesCh in the background so readLoop can progress; we're
	// asserting on the dirty-signal path, not the byte path.
	go func() {
		for range p.Bytes() {
		}
	}()

	// Wait for at least one feed-driven wake-up or for the channel
	// to close (readLoop finished). Both observably show the signal
	// plumbing works.
	select {
	case _, ok := <-sub.Ch:
		if !ok {
			// Closed before we saw an explicit feed-wake; the single
			// printf was so fast readLoop hit EOF before we observed
			// the signal. Closure itself proves the fan-out ran.
			return
		}
	case <-time.After(3 * time.Second):
		t.Fatal("subscription channel never fired after feed")
	}

	// Drain the rest via for-range; the channel must close when
	// readLoop exits.
	done := make(chan struct{})
	go func() {
		for range sub.Ch {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("subscription channel did not close after shell exit")
	}
}

func TestSubscribeAfterCloseIsClosed(t *testing.T) {
	p, err := Open(context.Background(), Config{
		Argv: []string{"/bin/sh", "-c", "printf hi"},
		Cols: 40, Rows: 5,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Drain bytesCh so readLoop progresses to EOF.
	go func() {
		for range p.Bytes() {
		}
	}()
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// After Close, new subscribers must get an already-closed channel
	// so for-range loops exit without blocking.
	sub := p.Subscribe()
	defer sub.Close()
	select {
	case _, ok := <-sub.Ch:
		if ok {
			// First receive might drain the prime signal, but the
			// next one must be a zero-value closed read.
			select {
			case _, ok2 := <-sub.Ch:
				if ok2 {
					t.Fatal("subscription on closed pane must close after priming")
				}
			case <-time.After(time.Second):
				t.Fatal("subscription on closed pane did not close")
			}
		}
	case <-time.After(time.Second):
		t.Fatal("subscription on closed pane blocked")
	}
}

func TestWriteAfterCloseErrClosed(t *testing.T) {
	p, err := Open(context.Background(), Config{
		Argv: []string{"/bin/cat"},
		Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	go func() {
		for range p.Bytes() {
		}
	}()

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := p.Write([]byte("x")); !errors.Is(err, ErrClosed) {
		t.Fatalf("expected errors.Is(err, ErrClosed) from Write, got %v", err)
	}
	if err := p.Resize(100, 30); !errors.Is(err, ErrClosed) {
		t.Fatalf("expected errors.Is(err, ErrClosed) from Resize, got %v", err)
	}
	if err := p.Signal(pty.SIGTERM); !errors.Is(err, ErrClosed) {
		t.Fatalf("expected errors.Is(err, ErrClosed) from Signal, got %v", err)
	}
}
