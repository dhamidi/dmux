package server

import (
	"fmt"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/session"
)

// respawnPane is a session.Pane that records Respawn/ClearHistory calls
// and can simulate an alive or exited process.
type respawnPane struct {
	id              session.PaneID
	pid             int // 0 means exited
	respawnCalled   bool
	respawnShell    string
	respawnErr      error
	historyCleared  bool
}

func (r *respawnPane) Title() string                                { return "respawn" }
func (r *respawnPane) Write(data []byte) error                     { return nil }
func (r *respawnPane) SendKey(key keys.Key) error                  { return nil }
func (r *respawnPane) Resize(cols, rows int) error                 { return nil }
func (r *respawnPane) Snapshot() pane.CellGrid                     { return pane.CellGrid{} }
func (r *respawnPane) CaptureContent(history bool) ([]byte, error) { return nil, nil }
func (r *respawnPane) Close() error                                { return nil }
func (r *respawnPane) ShellPID() int                               { return r.pid }
func (r *respawnPane) LastOutputAt() time.Time                     { return time.Time{} }
func (r *respawnPane) ConsumeBell() bool                           { return false }
func (r *respawnPane) ClearHistory()                               { r.historyCleared = true }
func (r *respawnPane) ClearScreen() error                          { return nil }
func (r *respawnPane) Respawn(shell string) error {
	r.respawnCalled = true
	r.respawnShell = shell
	return r.respawnErr
}

func newTestMutatorWithRespawnPane(pid int) (*serverMutator, *respawnPane) {
	state := session.NewServer()
	rp := &respawnPane{pid: pid}
	m := &serverMutator{
		state:    state,
		shutdown: func() {},
		newPane: func(cfg pane.Config) (session.Pane, error) {
			rp.id = cfg.ID
			return rp, nil
		},
	}
	sv, _ := m.NewSession("s1")
	_, _ = m.NewWindow(sv.ID, "w1")
	return m, rp
}

// TestRespawnPane_ExitedPane verifies that a pane with an exited process (PID=0)
// can be respawned without the -k flag.
func TestRespawnPane_ExitedPane(t *testing.T) {
	m, rp := newTestMutatorWithRespawnPane(0) // PID=0 means exited

	if err := m.RespawnPane(int(rp.id), "/bin/sh", false, false); err != nil {
		t.Fatalf("RespawnPane: %v", err)
	}
	if !rp.respawnCalled {
		t.Error("expected Respawn to be called, but it was not")
	}
	if rp.respawnShell != "/bin/sh" {
		t.Errorf("Respawn shell = %q, want %q", rp.respawnShell, "/bin/sh")
	}
}

// TestRespawnPane_ActivePaneWithoutKill verifies that respawning an active pane
// (PID > 0 and process alive) without the -k flag returns an error.
func TestRespawnPane_ActivePaneWithoutKill(t *testing.T) {
	// Use PID 1 (init/systemd) which is always alive on Linux.
	m, rp := newTestMutatorWithRespawnPane(1)

	err := m.RespawnPane(int(rp.id), "", false, false)
	if err == nil {
		t.Fatal("expected error for active pane without -k, got nil")
	}
	if err.Error() != "pane still active" {
		t.Errorf("error = %q, want %q", err.Error(), "pane still active")
	}
	if rp.respawnCalled {
		t.Error("Respawn should not have been called for active pane without -k")
	}
}

// TestRespawnPane_ClearsHistory verifies that scrollback is cleared when keepHistory=false.
func TestRespawnPane_ClearsHistory(t *testing.T) {
	m, rp := newTestMutatorWithRespawnPane(0)

	if err := m.RespawnPane(int(rp.id), "", false, false); err != nil {
		t.Fatalf("RespawnPane: %v", err)
	}
	if !rp.historyCleared {
		t.Error("expected ClearHistory to be called when keepHistory=false")
	}
}

// TestRespawnPane_KeepsHistory verifies that scrollback is preserved when keepHistory=true.
func TestRespawnPane_KeepsHistory(t *testing.T) {
	m, rp := newTestMutatorWithRespawnPane(0)

	if err := m.RespawnPane(int(rp.id), "", false, true); err != nil {
		t.Fatalf("RespawnPane: %v", err)
	}
	if rp.historyCleared {
		t.Error("ClearHistory should not be called when keepHistory=true")
	}
}

// TestRespawnPane_PaneNotFound verifies that an error is returned for unknown pane IDs.
func TestRespawnPane_PaneNotFound(t *testing.T) {
	m, _ := newTestMutatorWithRespawnPane(0)

	err := m.RespawnPane(9999, "", false, false)
	if err == nil {
		t.Fatal("expected error for unknown pane ID, got nil")
	}
}

// TestRespawnPane_PropagatesRespawnError verifies that errors from pane.Respawn are wrapped.
func TestRespawnPane_PropagatesRespawnError(t *testing.T) {
	m, rp := newTestMutatorWithRespawnPane(0)
	rp.respawnErr = fmt.Errorf("no PTY factory")

	err := m.RespawnPane(int(rp.id), "", false, false)
	if err == nil {
		t.Fatal("expected error from Respawn, got nil")
	}
}
