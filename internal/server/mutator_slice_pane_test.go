package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/session"
)

// ─── SlicePane tests ──────────────────────────────────────────────────────────

func TestSlicePane_SplitsTargetPane(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv, err := m.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	activePaneID := wv.Active

	pv, err := m.SlicePane(sv.ID, wv.ID, activePaneID)
	if err != nil {
		t.Fatalf("SlicePane: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	win := findWindowByID(sess, session.WindowID(wv.ID))

	if got := len(win.Panes); got != 2 {
		t.Errorf("pane count = %d, want 2", got)
	}

	if pv.ID == activePaneID {
		t.Errorf("new pane ID %d should differ from original pane ID", pv.ID)
	}
}

func TestSlicePane_ReturnsNewPaneView(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, _ := m.NewSession("s1")
	wv, _ := m.NewWindow(sv.ID, "win1")

	pv, err := m.SlicePane(sv.ID, wv.ID, wv.Active)
	if err != nil {
		t.Fatalf("SlicePane: %v", err)
	}

	if pv.ID == 0 {
		t.Error("expected non-zero pane ID in returned PaneView")
	}
}

func TestSlicePane_NotifiesPTYDimensions(t *testing.T) {
	m, panes, sessID, winID, _, pane1ID := newTestMutatorWithTracking(t)

	// Clear resize records from setup.
	for _, tp := range panes {
		tp.resizes = nil
	}

	_, err := m.SlicePane(sessID, winID, pane1ID)
	if err != nil {
		t.Fatalf("SlicePane: %v", err)
	}

	// At least the split pane should have received a Resize call.
	anyResized := false
	for _, tp := range panes {
		if len(tp.resizes) > 0 {
			anyResized = true
			break
		}
	}
	if !anyResized {
		t.Error("expected at least one Resize call after SlicePane, got none")
	}
}

func TestSlicePane_PaneNotFound(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, _ := m.NewSession("s1")
	wv, _ := m.NewWindow(sv.ID, "win1")

	if _, err := m.SlicePane(sv.ID, wv.ID, 9999); err == nil {
		t.Error("expected error for non-existent pane, got nil")
	}
}

func TestSlicePane_WindowNotFound(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, _ := m.NewSession("s1")
	wv, _ := m.NewWindow(sv.ID, "win1")

	if _, err := m.SlicePane(sv.ID, "no-such-window", wv.Active); err == nil {
		t.Error("expected error for non-existent window, got nil")
	}
}

func TestSlicePane_SessionNotFound(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, _ := m.NewSession("s1")
	wv, _ := m.NewWindow(sv.ID, "win1")

	if _, err := m.SlicePane("no-such-session", wv.ID, wv.Active); err == nil {
		t.Error("expected error for non-existent session, got nil")
	}
}

func TestSlicePane_MultipleSlices(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, _ := m.NewSession("s1")
	wv, _ := m.NewWindow(sv.ID, "win1")

	activePaneID := wv.Active

	// Slice the same pane twice.
	_, err := m.SlicePane(sv.ID, wv.ID, activePaneID)
	if err != nil {
		t.Fatalf("first SlicePane: %v", err)
	}
	_, err = m.SlicePane(sv.ID, wv.ID, activePaneID)
	if err != nil {
		t.Fatalf("second SlicePane: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	win := findWindowByID(sess, session.WindowID(wv.ID))

	if got := len(win.Panes); got != 3 {
		t.Errorf("pane count = %d, want 3", got)
	}
}
