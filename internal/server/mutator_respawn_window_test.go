package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/session"
)

// newTestMutatorForRespawnWindow creates a mutator with a session and window
// containing panes constructed from the given pids (one per pid).
// It returns the mutator, the session view, the window view, and the panes in order.
func newTestMutatorForRespawnWindow(pids ...int) (*serverMutator, string, string, []*respawnPane) {
	state := session.NewServer()
	panes := make([]*respawnPane, len(pids))
	callCount := 0

	m := &serverMutator{
		state:    state,
		shutdown: func() {},
		newPane: func(cfg pane.Config) (session.Pane, error) {
			rp := &respawnPane{id: cfg.ID, pid: pids[callCount]}
			panes[callCount] = rp
			callCount++
			return rp, nil
		},
	}

	sv, _ := m.NewSession("s1")
	wv, _ := m.NewWindow(sv.ID, "w1")

	// Add additional panes directly to the window for multi-pane scenarios.
	sess := state.Sessions[session.SessionID(sv.ID)]
	var win *session.Window
	for _, wl := range sess.Windows {
		if string(wl.Window.ID) == wv.ID {
			win = wl.Window
			break
		}
	}
	for i := 1; i < len(pids); i++ {
		rp := &respawnPane{pid: pids[i]}
		panes[i] = rp
		win.AddPane(session.PaneID(100+i), rp)
	}

	return m, sv.ID, wv.ID, panes
}

// TestRespawnWindow_AllPanesExited verifies that a window whose panes have all
// exited (PID=0) can be respawned without -k.
func TestRespawnWindow_AllPanesExited(t *testing.T) {
	m, sessID, winID, panes := newTestMutatorForRespawnWindow(0, 0)

	if err := m.RespawnWindow(sessID, winID, "/bin/sh", "", false, false); err != nil {
		t.Fatalf("RespawnWindow: %v", err)
	}
	for i, rp := range panes {
		if !rp.respawnCalled {
			t.Errorf("pane[%d]: expected Respawn to be called", i)
		}
		if rp.respawnShell != "/bin/sh" {
			t.Errorf("pane[%d]: Respawn shell = %q, want %q", i, rp.respawnShell, "/bin/sh")
		}
	}
}

// TestRespawnWindow_SomeAliveWithoutKill verifies that if any pane is still
// alive and -k is not set, RespawnWindow returns an error without respawning.
func TestRespawnWindow_SomeAliveWithoutKill(t *testing.T) {
	// PID=0 means exited, PID=1 is always alive on Linux (init/systemd).
	m, sessID, winID, panes := newTestMutatorForRespawnWindow(0, 1)

	err := m.RespawnWindow(sessID, winID, "", "", false, false)
	if err == nil {
		t.Fatal("expected error when a pane is still active, got nil")
	}
	if err.Error() != "pane still active" {
		t.Errorf("error = %q, want %q", err.Error(), "pane still active")
	}
	for i, rp := range panes {
		if rp.respawnCalled {
			t.Errorf("pane[%d]: Respawn should not have been called", i)
		}
	}
}

// TestRespawnWindow_AllKilledWithK verifies that all panes are respawned when
// -k is set even if some are still alive.
func TestRespawnWindow_AllKilledWithK(t *testing.T) {
	// PID=1 is always alive on Linux.
	m, sessID, winID, panes := newTestMutatorForRespawnWindow(1, 1)

	if err := m.RespawnWindow(sessID, winID, "/bin/bash", "", true, false); err != nil {
		t.Fatalf("RespawnWindow with -k: %v", err)
	}
	for i, rp := range panes {
		if !rp.respawnCalled {
			t.Errorf("pane[%d]: expected Respawn to be called with -k", i)
		}
	}
}

// TestRespawnWindow_ClearsHistory verifies scrollback is cleared when keepHistory=false.
func TestRespawnWindow_ClearsHistory(t *testing.T) {
	m, sessID, winID, panes := newTestMutatorForRespawnWindow(0, 0)

	if err := m.RespawnWindow(sessID, winID, "", "", false, false); err != nil {
		t.Fatalf("RespawnWindow: %v", err)
	}
	for i, rp := range panes {
		if !rp.historyCleared {
			t.Errorf("pane[%d]: expected ClearHistory to be called when keepHistory=false", i)
		}
	}
}

// TestRespawnWindow_KeepsHistory verifies scrollback is preserved when keepHistory=true.
func TestRespawnWindow_KeepsHistory(t *testing.T) {
	m, sessID, winID, panes := newTestMutatorForRespawnWindow(0, 0)

	if err := m.RespawnWindow(sessID, winID, "", "", false, true); err != nil {
		t.Fatalf("RespawnWindow: %v", err)
	}
	for i, rp := range panes {
		if rp.historyCleared {
			t.Errorf("pane[%d]: ClearHistory should not be called when keepHistory=true", i)
		}
	}
}

// TestRespawnWindow_WindowNotFound verifies an error is returned for unknown windows.
func TestRespawnWindow_WindowNotFound(t *testing.T) {
	m, sessID, _, _ := newTestMutatorForRespawnWindow(0)

	err := m.RespawnWindow(sessID, "no-such-window", "", "", false, false)
	if err == nil {
		t.Fatal("expected error for unknown window, got nil")
	}
}

// TestRespawnWindow_SessionNotFound verifies an error is returned for unknown sessions.
func TestRespawnWindow_SessionNotFound(t *testing.T) {
	m, _, winID, _ := newTestMutatorForRespawnWindow(0)

	err := m.RespawnWindow("no-such-session", winID, "", "", false, false)
	if err == nil {
		t.Fatal("expected error for unknown session, got nil")
	}
}
