package server

import (
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/session"
)

// ─── pipePaneTest: mock pane that implements session.Pane + pipablePane ───────

type pipePaneTest struct {
	id          session.PaneID
	hasPipe     bool
	shellCmds   []string // commands passed to AttachPipe
	detachCount int      // number of DetachPipe calls
	attachErr   error    // error to return from AttachPipe
}

// session.Pane stubs.
func (p *pipePaneTest) Title() string                                { return "pipe-test" }
func (p *pipePaneTest) Write(data []byte) error                     { return nil }
func (p *pipePaneTest) SendKey(key keys.Key) error                  { return nil }
func (p *pipePaneTest) Resize(cols, rows int) error                 { return nil }
func (p *pipePaneTest) Snapshot() pane.CellGrid                     { return pane.CellGrid{} }
func (p *pipePaneTest) CaptureContent(history bool) ([]byte, error) { return nil, nil }
func (p *pipePaneTest) Respawn(shell string) error                  { return nil }
func (p *pipePaneTest) Close() error                                { return nil }
func (p *pipePaneTest) ShellPID() int                               { return 0 }
func (p *pipePaneTest) LastOutputAt() time.Time                     { return time.Time{} }
func (p *pipePaneTest) ConsumeBell() bool                           { return false }
func (p *pipePaneTest) ClearHistory()                               {}
func (p *pipePaneTest) ClearScreen() error                          { return nil }

// pipablePane implementation.
func (p *pipePaneTest) HasPipe() bool { return p.hasPipe }
func (p *pipePaneTest) AttachPipe(shellCmd string) error {
	if p.attachErr != nil {
		return p.attachErr
	}
	p.hasPipe = true
	p.shellCmds = append(p.shellCmds, shellCmd)
	return nil
}
func (p *pipePaneTest) DetachPipe() error {
	p.hasPipe = false
	p.detachCount++
	return nil
}

// newTestMutatorWithPipePaneTest creates a minimal mutator with a single
// pipePaneTest in the default session/window.
func newTestMutatorWithPipePaneTest() (*serverMutator, *pipePaneTest) {
	state := session.NewServer()
	pt := &pipePaneTest{}
	m := &serverMutator{
		state:    state,
		shutdown: func() {},
		newPane: func(cfg pane.Config) (session.Pane, error) {
			pt.id = cfg.ID
			return pt, nil
		},
	}
	sv, _ := m.NewSession("s1")
	_, _ = m.NewWindow(sv.ID, "w1")
	return m, pt
}

// ─── tests ────────────────────────────────────────────────────────────────────

// TestPipePane_AttachesPipeWithCommand verifies that PipePane with a shell
// command calls AttachPipe on the target pane.
func TestPipePane_AttachesPipeWithCommand(t *testing.T) {
	m, pt := newTestMutatorWithPipePaneTest()

	if err := m.PipePane(int(pt.id), "cat", false, true, false); err != nil {
		t.Fatalf("PipePane: %v", err)
	}
	if len(pt.shellCmds) != 1 || pt.shellCmds[0] != "cat" {
		t.Errorf("AttachPipe called with %v, want [cat]", pt.shellCmds)
	}
	if !pt.hasPipe {
		t.Error("HasPipe = false after PipePane with command, want true")
	}
}

// TestPipePane_EmptyCommandDetaches verifies that PipePane with an empty shell
// command stops any existing pipe (toggle-off behaviour).
func TestPipePane_EmptyCommandDetaches(t *testing.T) {
	m, pt := newTestMutatorWithPipePaneTest()

	// First attach a pipe.
	_ = m.PipePane(int(pt.id), "cat", false, true, false)

	// Then call with empty command to stop it.
	if err := m.PipePane(int(pt.id), "", false, true, false); err != nil {
		t.Fatalf("PipePane stop: %v", err)
	}
	if pt.hasPipe {
		t.Error("HasPipe = true after PipePane with empty command, want false")
	}
}

// TestPipePane_DetachesBeforeReattaching verifies that PipePane stops an
// existing pipe before attaching a new one.
func TestPipePane_DetachesBeforeReattaching(t *testing.T) {
	m, pt := newTestMutatorWithPipePaneTest()

	_ = m.PipePane(int(pt.id), "cat", false, true, false)
	initialDetach := pt.detachCount

	_ = m.PipePane(int(pt.id), "tee /dev/null", false, true, false)

	if pt.detachCount <= initialDetach {
		t.Error("DetachPipe not called before attaching a new pipe")
	}
	if len(pt.shellCmds) != 2 {
		t.Errorf("AttachPipe call count = %d, want 2", len(pt.shellCmds))
	}
}

// TestPipePane_OnceFlagSkipsWhenAlreadyPiped verifies that the -o flag leaves
// an existing pipe untouched.
func TestPipePane_OnceFlagSkipsWhenAlreadyPiped(t *testing.T) {
	m, pt := newTestMutatorWithPipePaneTest()

	// Attach initial pipe.
	_ = m.PipePane(int(pt.id), "cat", false, true, false)
	firstCmds := len(pt.shellCmds)

	// Call again with onceFlag=true; should be a no-op.
	if err := m.PipePane(int(pt.id), "tee /dev/null", false, true, true); err != nil {
		t.Fatalf("PipePane -o: %v", err)
	}
	if len(pt.shellCmds) != firstCmds {
		t.Errorf("AttachPipe called %d times with -o flag when already piped, want %d", len(pt.shellCmds), firstCmds)
	}
}

// TestPipePane_OnceFlagAttachesWhenNotPiped verifies that the -o flag attaches
// a pipe when no pipe is currently active.
func TestPipePane_OnceFlagAttachesWhenNotPiped(t *testing.T) {
	m, pt := newTestMutatorWithPipePaneTest()

	if err := m.PipePane(int(pt.id), "cat", false, true, true); err != nil {
		t.Fatalf("PipePane -o on unpiped pane: %v", err)
	}
	if !pt.hasPipe {
		t.Error("HasPipe = false after PipePane -o on unpiped pane")
	}
}

// TestPipePane_PaneNotFound verifies that an error is returned for an unknown pane ID.
func TestPipePane_PaneNotFound(t *testing.T) {
	m, _ := newTestMutatorWithPipePaneTest()

	err := m.PipePane(9999, "cat", false, true, false)
	if err == nil {
		t.Fatal("expected error for unknown pane ID, got nil")
	}
}
