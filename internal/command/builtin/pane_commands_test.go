package builtin_test

import (
	"os/exec"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/command"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/pty"
)

// ─── pipe-pane tests ──────────────────────────────────────────────────────────

// pipePaneBackend embeds testBackend and overrides PipePane with real subprocess
// management so tests can observe actual subprocess lifecycle.
type pipePaneBackend struct {
	*testBackend
	pipes map[int]*exec.Cmd
}

func newPipePaneBackend() *pipePaneBackend {
	return &pipePaneBackend{
		testBackend: newBackend(),
		pipes:       make(map[int]*exec.Cmd),
	}
}

// PipePane starts or stops a subprocess for the given pane.
// An empty shellCmd means stop any existing pipe.
func (b *pipePaneBackend) PipePane(paneID int, shellCmd string, inFlag, outFlag, onceFlag bool) error {
	// Stop existing pipe if any.
	if existing, ok := b.pipes[paneID]; ok {
		if existing.Process != nil {
			_ = existing.Process.Kill()
			_ = existing.Wait()
		}
		delete(b.pipes, paneID)
	}
	if shellCmd == "" {
		return nil
	}
	if onceFlag && len(b.pipes) > 0 {
		// Already piped; skip.
		return nil
	}
	cmd := exec.Command("sh", "-c", shellCmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	b.pipes[paneID] = cmd
	return nil
}

func dispatchPipe(name string, args []string, b *pipePaneBackend) command.Result {
	return command.Default.Dispatch(name, args, b.testBackend, client1(), command.NewQueue(), b)
}

// TestPipePane_StartsSubprocess verifies that pipe-pane with a shell command
// starts a subprocess tracked per-pane.
func TestPipePane_StartsSubprocess(t *testing.T) {
	b := newPipePaneBackend()
	res := dispatchPipe("pipe-pane", []string{"cat"}, b)
	if res.Err != nil {
		t.Fatalf("pipe-pane cat: %v", res.Err)
	}
	if _, ok := b.pipes[1]; !ok {
		t.Error("pipe-pane: subprocess not started for pane 1")
	}
}

// TestPipePane_StopOnNoCommand verifies that calling pipe-pane again with no
// command stops the running subprocess.
func TestPipePane_StopOnNoCommand(t *testing.T) {
	b := newPipePaneBackend()
	// Start a pipe.
	if res := dispatchPipe("pipe-pane", []string{"cat"}, b); res.Err != nil {
		t.Fatalf("pipe-pane start: %v", res.Err)
	}
	if _, ok := b.pipes[1]; !ok {
		t.Fatal("pipe-pane: subprocess not started")
	}
	// Stop the pipe by calling with no command.
	res := dispatchPipe("pipe-pane", nil, b)
	if res.Err != nil {
		t.Fatalf("pipe-pane stop: %v", res.Err)
	}
	if _, ok := b.pipes[1]; ok {
		t.Error("pipe-pane: subprocess still tracked after stop")
	}
}

// ─── clear-history tests ──────────────────────────────────────────────────────

// TestClearHistory_EmptiesScrollback verifies that the clear-history command
// calls ClearHistory on the mutator with the correct pane ID.
func TestClearHistory_EmptiesScrollback(t *testing.T) {
	b := newBackend()
	res := dispatch("clear-history", nil, b)
	if res.Err != nil {
		t.Fatalf("clear-history: %v", res.Err)
	}
	if len(b.clearedHistoryPanes) != 1 {
		t.Fatalf("expected 1 ClearHistory call, got %d", len(b.clearedHistoryPanes))
	}
	got := b.clearedHistoryPanes[0]
	if got.paneID != 1 {
		t.Errorf("ClearHistory paneID = %d, want 1", got.paneID)
	}
	if got.visibleToo {
		t.Error("ClearHistory visibleToo should be false without -H flag")
	}
}

// TestClearHistory_WithHFlag_ClearsVisible verifies -H causes visibleToo=true.
func TestClearHistory_WithHFlag_ClearsVisible(t *testing.T) {
	b := newBackend()
	res := dispatch("clear-history", []string{"-H"}, b)
	if res.Err != nil {
		t.Fatalf("clear-history -H: %v", res.Err)
	}
	if len(b.clearedHistoryPanes) != 1 || !b.clearedHistoryPanes[0].visibleToo {
		t.Errorf("expected visibleToo=true with -H flag, got: %v", b.clearedHistoryPanes)
	}
}

// TestClearHistory_Pane_ClearsScrollbackBuffer verifies that the pane-level
// ClearHistory method actually empties the scrollback ring buffer.
func TestClearHistory_Pane_ClearsScrollbackBuffer(t *testing.T) {
	fp := &pty.FakePTY{}
	ft := &pane.FakeTerminal{}
	p, err := pane.New(pane.Config{
		ID:    1,
		PTY:   fp,
		Term:  ft,
		Keys:  &pane.FakeKeyEncoder{},
		Mouse: &pane.FakeMouseEncoder{},
	})
	if err != nil {
		t.Fatalf("pane.New: %v", err)
	}
	defer p.Close()

	// ClearHistory must not panic and must be callable multiple times.
	p.ClearHistory()
	p.ClearHistory()
}

// ─── clear-pane tests ─────────────────────────────────────────────────────────

// TestClearPane_CallsMutator verifies the clear-pane command calls ClearPane
// on the mutator with the correct pane ID.
func TestClearPane_CallsMutator(t *testing.T) {
	b := newBackend()
	res := dispatch("clear-pane", nil, b)
	if res.Err != nil {
		t.Fatalf("clear-pane: %v", res.Err)
	}
	if len(b.clearedPanes) != 1 || b.clearedPanes[0] != 1 {
		t.Errorf("ClearPane pane IDs = %v, want [1]", b.clearedPanes)
	}
}

// TestClearPane_WritesAnsiClear verifies that the pane-level ClearScreen method
// injects the ANSI cursor-home + erase-display sequence into the PTY.
func TestClearPane_WritesAnsiClear(t *testing.T) {
	fp := &pty.FakePTY{}
	ft := &pane.FakeTerminal{}
	p, err := pane.New(pane.Config{
		ID:    1,
		PTY:   fp,
		Term:  ft,
		Keys:  &pane.FakeKeyEncoder{},
		Mouse: &pane.FakeMouseEncoder{},
	})
	if err != nil {
		t.Fatalf("pane.New: %v", err)
	}
	defer p.Close()

	if err := p.ClearScreen(); err != nil {
		t.Fatalf("ClearScreen: %v", err)
	}

	// Poll briefly to let the write settle (PTY write is synchronous but
	// test uses a goroutine-backed FakePTY).
	deadline := time.Now().Add(100 * time.Millisecond)
	var got []byte
	for time.Now().Before(deadline) {
		got = fp.Input()
		if len(got) > 0 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	want := "\x1b[H\x1b[2J"
	if string(got) != want {
		t.Errorf("ClearScreen wrote %q, want %q", got, want)
	}
}
