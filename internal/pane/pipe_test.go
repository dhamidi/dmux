// White-box tests for pipe attachment (package pane, not pane_test).
// These tests access unexported struct fields directly so they can bypass
// exec.Command and verify the copyOutput → pipeWriter data path in isolation.
package pane

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/pty"
)

// TestCopyOutput_ForwardsToPipeWriter verifies that bytes read from the PTY are
// also written to pipeWriter when one is set, in addition to the terminal.
// Data is pre-loaded into the FakePTY before the goroutine starts so there is
// no race between the goroutine's first Read call and InjectOutput.
func TestCopyOutput_ForwardsToPipeWriter(t *testing.T) {
	pr, pw := io.Pipe()

	data := []byte("hello pipe")
	fp := &pty.FakePTY{}
	// Pre-load output BEFORE starting the goroutine to avoid a race where the
	// goroutine reads an empty buffer (returning io.EOF) and exits.
	fp.InjectOutput(data)

	ft := &FakeTerminal{}
	p := &pane{
		id:         1,
		ptyField:   fp,
		term:       ft,
		keyEnc:     &FakeKeyEncoder{},
		mouseEnc:   &FakeMouseEncoder{},
		pipeWriter: pw,
		done:       make(chan struct{}),
	}
	go p.copyOutput()

	// Read from the other end of the pipe.
	received := make([]byte, len(data))
	readDone := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(pr, received)
		readDone <- err
	}()

	select {
	case err := <-readDone:
		if err != nil {
			t.Fatalf("pipe read: %v", err)
		}
		if !bytes.Equal(received, data) {
			t.Errorf("pipe received %q, want %q", received, data)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout: PTY output did not arrive in pipe writer")
	}

	// Verify the terminal also received the bytes.
	if got := ft.Written(); !bytes.Equal(got, data) {
		t.Errorf("terminal received %q, want %q", got, data)
	}

	pw.Close() //nolint:errcheck
	fp.Close() //nolint:errcheck
	<-p.done
}

// TestCopyOutput_NoPipeWriter verifies that copyOutput works normally when no
// pipeWriter is set (regression guard).
func TestCopyOutput_NoPipeWriter(t *testing.T) {
	fp := &pty.FakePTY{}
	ft := &FakeTerminal{}
	p := &pane{
		id:       1,
		ptyField: fp,
		term:     ft,
		keyEnc:   &FakeKeyEncoder{},
		mouseEnc: &FakeMouseEncoder{},
		done:     make(chan struct{}),
	}
	go p.copyOutput()

	data := []byte("no pipe output")
	fp.InjectOutput(data)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if bytes.Equal(ft.Written(), data) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := ft.Written(); !bytes.Equal(got, data) {
		t.Errorf("terminal received %q, want %q", got, data)
	}

	fp.Close() //nolint:errcheck
	<-p.done
}
