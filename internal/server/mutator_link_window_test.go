package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/session"
)

// TestLinkWindow_WindowAppearsInBothSessions verifies that after link-window
// the same window appears in both the source and destination session.
func TestLinkWindow_WindowAppearsInBothSessions(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv1, err := m.NewSession("src")
	if err != nil {
		t.Fatalf("NewSession src: %v", err)
	}
	sv2, err := m.NewSession("dst")
	if err != nil {
		t.Fatalf("NewSession dst: %v", err)
	}

	wv, err := m.NewWindow(sv1.ID, "shared")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	if err := m.LinkWindow(sv1.ID, wv.ID, sv2.ID, -1, false, false, true, false); err != nil {
		t.Fatalf("LinkWindow: %v", err)
	}

	srcSess := m.state.Sessions[session.SessionID(sv1.ID)]
	dstSess := m.state.Sessions[session.SessionID(sv2.ID)]

	// The window must appear in the source session.
	var srcWin *session.Window
	for _, wl := range srcSess.Windows {
		if string(wl.Window.ID) == wv.ID {
			srcWin = wl.Window
		}
	}
	if srcWin == nil {
		t.Fatal("window not found in source session after link-window")
	}

	// The window must also appear in the destination session.
	var dstWin *session.Window
	for _, wl := range dstSess.Windows {
		if string(wl.Window.ID) == wv.ID {
			dstWin = wl.Window
		}
	}
	if dstWin == nil {
		t.Fatal("window not found in destination session after link-window")
	}

	// Both must be the same pointer.
	if srcWin != dstWin {
		t.Error("source and destination winlinks point to different window objects")
	}

	// window_linked should be reported as true.
	if got, _ := srcWin.Lookup("window_linked"); got != "1" {
		t.Errorf("window_linked = %q, want %q", got, "1")
	}
	if got, _ := srcWin.Lookup("window_linked_sessions"); got != "2" {
		t.Errorf("window_linked_sessions = %q, want %q", got, "2")
	}
}

// TestUnlinkWindow_NoKill_LeavesWindowAlive verifies that unlink-window without
// -k removes the window from the session but leaves it alive in the other session.
func TestUnlinkWindow_NoKill_LeavesWindowAlive(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv1, _ := m.NewSession("src")
	sv2, _ := m.NewSession("dst")
	wv, _ := m.NewWindow(sv1.ID, "shared")

	if err := m.LinkWindow(sv1.ID, wv.ID, sv2.ID, -1, false, false, true, false); err != nil {
		t.Fatalf("LinkWindow: %v", err)
	}

	// Unlink from the destination session without -k.
	if err := m.UnlinkWindow(sv2.ID, wv.ID, false); err != nil {
		t.Fatalf("UnlinkWindow: %v", err)
	}

	// Window must still exist in the source session.
	srcSess := m.state.Sessions[session.SessionID(sv1.ID)]
	found := false
	for _, wl := range srcSess.Windows {
		if string(wl.Window.ID) == wv.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("window disappeared from source session after unlink-window without -k")
	}

	// Window must NOT appear in the destination session.
	dstSess := m.state.Sessions[session.SessionID(sv2.ID)]
	for _, wl := range dstSess.Windows {
		if string(wl.Window.ID) == wv.ID {
			t.Fatal("window still present in destination session after unlink-window")
		}
	}
}

// TestUnlinkWindow_WithKill_KillsWindowWhenNoSessionsRemain verifies that
// unlink-window -k closes all panes when the window has no remaining sessions.
func TestUnlinkWindow_WithKill_KillsWindowWhenNoSessionsRemain(t *testing.T) {
	m, _, created := newTestMutatorWithPane()

	sv1, _ := m.NewSession("src")
	sv2, _ := m.NewSession("dst")
	wv, _ := m.NewWindow(sv1.ID, "shared")

	// Link the window into the destination session.
	if err := m.LinkWindow(sv1.ID, wv.ID, sv2.ID, -1, false, false, true, false); err != nil {
		t.Fatalf("LinkWindow: %v", err)
	}

	// Unlink from source (so dst is only remaining session).
	if err := m.UnlinkWindow(sv1.ID, wv.ID, false); err != nil {
		t.Fatalf("UnlinkWindow from src: %v", err)
	}

	// Now unlink from destination with -k.
	if err := m.UnlinkWindow(sv2.ID, wv.ID, true); err != nil {
		t.Fatalf("UnlinkWindow from dst -k: %v", err)
	}

	// All panes created for this window must have been closed.
	for _, fp := range *created {
		if !fp.closed {
			t.Errorf("pane %v was not closed after unlink-window -k", fp.id)
		}
	}
}
