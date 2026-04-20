package server

import (
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/session"
)

// trackingPane records Resize calls and implements session.Pane.
type trackingPane struct {
	id      session.PaneID
	resizes []struct{ cols, rows int }
}

func (p *trackingPane) Title() string                                { return "tracking" }
func (p *trackingPane) Write(data []byte) error                      { return nil }
func (p *trackingPane) SendKey(key keys.Key) error                   { return nil }
func (p *trackingPane) Resize(cols, rows int) error {
	p.resizes = append(p.resizes, struct{ cols, rows int }{cols, rows})
	return nil
}
func (p *trackingPane) Snapshot() pane.CellGrid                     { return pane.CellGrid{} }
func (p *trackingPane) CaptureContent(history bool) ([]byte, error) { return nil, nil }
func (p *trackingPane) Respawn(shell string) error                   { return nil }
func (p *trackingPane) Close() error                                 { return nil }
func (p *trackingPane) ShellPID() int                                { return 0 }
func (p *trackingPane) LastOutputAt() time.Time                      { return time.Time{} }
func (p *trackingPane) ConsumeBell() bool                            { return false }
func (p *trackingPane) ClearHistory()                                {}
func (p *trackingPane) ClearScreen() error                           { return nil }

// newTestMutatorWithTracking creates a serverMutator that returns trackingPane
// instances and a window split into two panes vertically (80×24).
// Returns the mutator, created panes (index 0 = first, 1 = second),
// the session ID, window ID, and both pane IDs.
func newTestMutatorWithTracking(t *testing.T) (
	m *serverMutator,
	panes []*trackingPane,
	sessID, winID string,
	pane1ID, pane2ID int,
) {
	t.Helper()
	state := session.NewServer()
	var created []*trackingPane
	mut := &serverMutator{
		state:    state,
		shutdown: func() {},
		newPane: func(cfg pane.Config) (session.Pane, error) {
			tp := &trackingPane{id: cfg.ID}
			created = append(created, tp)
			return tp, nil
		},
	}

	sv, err := mut.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv, err := mut.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	sess := mut.state.Sessions[session.SessionID(sv.ID)]
	win := sess.Windows[0].Window
	var p1ID int
	for id := range win.Panes {
		p1ID = int(id)
		break
	}

	pv, err := mut.SplitWindow(sv.ID, wv.ID)
	if err != nil {
		t.Fatalf("SplitWindow: %v", err)
	}

	// Clear resize records from setup.
	for _, tp := range created {
		tp.resizes = nil
	}

	return mut, created, sv.ID, wv.ID, p1ID, pv.ID
}

// ─── SwapPane tests ───────────────────────────────────────────────────────────

func TestSwapPane_SwapsLeafPositions(t *testing.T) {
	m, _, sessID, winID, pane1ID, pane2ID := newTestMutatorWithTracking(t)

	sess := m.state.Sessions[session.SessionID(sessID)]
	win := sess.Windows[0].Window

	r1before := win.Layout.Rect(session.PaneID(pane1ID))
	r2before := win.Layout.Rect(session.PaneID(pane2ID))

	if err := m.SwapPane(sessID, winID, pane1ID, pane2ID); err != nil {
		t.Fatalf("SwapPane: %v", err)
	}

	r1after := win.Layout.Rect(session.PaneID(pane1ID))
	r2after := win.Layout.Rect(session.PaneID(pane2ID))

	if r1after != r2before {
		t.Errorf("pane1 rect after swap = %+v, want %+v", r1after, r2before)
	}
	if r2after != r1before {
		t.Errorf("pane2 rect after swap = %+v, want %+v", r2after, r1before)
	}
}

func TestSwapPane_NotifiesPTYs(t *testing.T) {
	m, panes, sessID, winID, pane1ID, pane2ID := newTestMutatorWithTracking(t)

	if err := m.SwapPane(sessID, winID, pane1ID, pane2ID); err != nil {
		t.Fatalf("SwapPane: %v", err)
	}

	// Both panes should have received a Resize call.
	for i, tp := range panes {
		if len(tp.resizes) == 0 {
			t.Errorf("pane index %d: expected Resize call after swap, got none", i)
			continue
		}
		last := tp.resizes[len(tp.resizes)-1]
		if last.cols <= 0 || last.rows <= 0 {
			t.Errorf("pane index %d: Resize called with non-positive dims cols=%d rows=%d", i, last.cols, last.rows)
		}
	}
}

func TestSwapPane_SamePane_NoOp(t *testing.T) {
	m, panes, sessID, winID, pane1ID, _ := newTestMutatorWithTracking(t)

	if err := m.SwapPane(sessID, winID, pane1ID, pane1ID); err != nil {
		t.Fatalf("SwapPane same pane: %v", err)
	}

	for _, tp := range panes {
		if len(tp.resizes) != 0 {
			t.Errorf("expected no Resize on no-op swap, got %d", len(tp.resizes))
		}
	}
}

func TestSwapPane_SessionNotFound(t *testing.T) {
	m, _, _, winID, pane1ID, pane2ID := newTestMutatorWithTracking(t)

	if err := m.SwapPane("no-such-session", winID, pane1ID, pane2ID); err == nil {
		t.Error("expected error for unknown session, got nil")
	}
}

func TestSwapPane_WindowNotFound(t *testing.T) {
	m, _, sessID, _, pane1ID, pane2ID := newTestMutatorWithTracking(t)

	if err := m.SwapPane(sessID, "no-such-window", pane1ID, pane2ID); err == nil {
		t.Error("expected error for unknown window, got nil")
	}
}

func TestSwapPane_PaneNotFound(t *testing.T) {
	m, _, sessID, winID, pane1ID, _ := newTestMutatorWithTracking(t)

	if err := m.SwapPane(sessID, winID, pane1ID, 9999); err == nil {
		t.Error("expected error for unknown pane, got nil")
	}
}

// ─── MovePane tests ───────────────────────────────────────────────────────────

// newTestMutatorWithTwoWindows creates a mutator with two separate windows
// (win1 split into two panes, win2 with one pane) in the same session.
func newTestMutatorWithTwoWindows(t *testing.T) (
	m *serverMutator,
	sessID, win1ID, win2ID string,
	win1pane1ID, win1pane2ID, win2pane1ID int,
) {
	t.Helper()
	state := session.NewServer()
	mut := &serverMutator{
		state:    state,
		shutdown: func() {},
		newPane: func(cfg pane.Config) (session.Pane, error) {
			return &trackingPane{id: cfg.ID}, nil
		},
	}

	sv, err := mut.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv1, err := mut.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow win1: %v", err)
	}
	wv2, err := mut.NewWindow(sv.ID, "win2")
	if err != nil {
		t.Fatalf("NewWindow win2: %v", err)
	}

	sess := mut.state.Sessions[session.SessionID(sv.ID)]

	// Get pane ID in win1 (initial pane created by NewWindow).
	win1 := findWindowByID(sess, session.WindowID(wv1.ID))
	var w1p1 int
	for id := range win1.Panes {
		w1p1 = int(id)
		break
	}

	// Split win1 to get a second pane.
	pv, err := mut.SplitWindow(sv.ID, wv1.ID)
	if err != nil {
		t.Fatalf("SplitWindow: %v", err)
	}

	// Get pane ID in win2.
	win2 := findWindowByID(sess, session.WindowID(wv2.ID))
	var w2p1 int
	for id := range win2.Panes {
		w2p1 = int(id)
		break
	}

	return mut, sv.ID, wv1.ID, wv2.ID, w1p1, pv.ID, w2p1
}

func findWindowByID(sess *session.Session, id session.WindowID) *session.Window {
	for _, wl := range sess.Windows {
		if wl.Window.ID == id {
			return wl.Window
		}
	}
	return nil
}

func TestMovePane_SameWindow(t *testing.T) {
	m, sessID, win1ID, _, pane1ID, pane2ID, _ := newTestMutatorWithTwoWindows(t)

	sess := m.state.Sessions[session.SessionID(sessID)]
	win1 := findWindowByID(sess, session.WindowID(win1ID))

	countBefore := len(win1.Panes)

	if err := m.MovePane(sessID, win1ID, pane2ID, sessID, win1ID); err != nil {
		t.Fatalf("MovePane same window: %v", err)
	}

	// Pane count in win1 should stay the same — one removed, one added.
	if got := len(win1.Panes); got != countBefore {
		t.Errorf("win1 pane count = %d, want %d", got, countBefore)
	}
	_ = pane1ID
}

func TestMovePane_CrossWindow(t *testing.T) {
	m, sessID, win1ID, win2ID, _, pane2ID, _ := newTestMutatorWithTwoWindows(t)

	sess := m.state.Sessions[session.SessionID(sessID)]
	win1 := findWindowByID(sess, session.WindowID(win1ID))
	win2 := findWindowByID(sess, session.WindowID(win2ID))

	win1Before := len(win1.Panes)
	win2Before := len(win2.Panes)

	if err := m.MovePane(sessID, win1ID, pane2ID, sessID, win2ID); err != nil {
		t.Fatalf("MovePane cross-window: %v", err)
	}

	if got := len(win1.Panes); got != win1Before-1 {
		t.Errorf("win1 pane count = %d, want %d", got, win1Before-1)
	}
	if got := len(win2.Panes); got != win2Before+1 {
		t.Errorf("win2 pane count = %d, want %d", got, win2Before+1)
	}
}

func TestMovePane_CrossWindow_LastPaneKillsSourceWindow(t *testing.T) {
	m, sessID, win1ID, win2ID, pane1ID, _, _ := newTestMutatorWithTwoWindows(t)

	// win1 has 2 panes; move one, leaving 1. Then move the last pane.
	sess := m.state.Sessions[session.SessionID(sessID)]
	win1 := findWindowByID(sess, session.WindowID(win1ID))

	// Move pane2 away first so only pane1 remains.
	var pane2ID int
	for id := range win1.Panes {
		if int(id) != pane1ID {
			pane2ID = int(id)
			break
		}
	}
	if err := m.MovePane(sessID, win1ID, pane2ID, sessID, win2ID); err != nil {
		t.Fatalf("first MovePane: %v", err)
	}

	// Now win1 should have exactly 1 pane. Move the last pane.
	if err := m.MovePane(sessID, win1ID, pane1ID, sessID, win2ID); err != nil {
		t.Fatalf("second MovePane: %v", err)
	}

	// win1 should no longer exist (killed because it became empty).
	win1After := findWindowByID(sess, session.WindowID(win1ID))
	if win1After != nil {
		t.Errorf("expected win1 to be killed after last pane moved out, but it still exists")
	}
}

func TestMovePane_SourceWindowNotFound(t *testing.T) {
	m, sessID, _, win2ID, _, pane2ID, _ := newTestMutatorWithTwoWindows(t)

	if err := m.MovePane(sessID, "no-such-window", pane2ID, sessID, win2ID); err == nil {
		t.Error("expected error for unknown source window, got nil")
	}
}

func TestMovePane_DstWindowNotFound(t *testing.T) {
	m, sessID, win1ID, _, _, pane2ID, _ := newTestMutatorWithTwoWindows(t)

	if err := m.MovePane(sessID, win1ID, pane2ID, sessID, "no-such-window"); err == nil {
		t.Error("expected error for unknown destination window, got nil")
	}
}

func TestMovePane_PaneNotFound(t *testing.T) {
	m, sessID, win1ID, win2ID, _, _, _ := newTestMutatorWithTwoWindows(t)

	if err := m.MovePane(sessID, win1ID, 9999, sessID, win2ID); err == nil {
		t.Error("expected error for unknown pane, got nil")
	}
}
