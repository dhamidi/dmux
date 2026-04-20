package server

import (
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/session"
)

// fakePane is a minimal session.Pane implementation for window/pane tests.
// closed is set to true when Close is called, allowing tests to verify teardown.
type fakePane struct {
	id     session.PaneID
	closed bool
}

func (f *fakePane) Title() string                               { return "fake" }
func (f *fakePane) Write(data []byte) error                    { return nil }
func (f *fakePane) SendKey(key keys.Key) error                 { return nil }
func (f *fakePane) Resize(cols, rows int) error                { return nil }
func (f *fakePane) Snapshot() pane.CellGrid                   { return pane.CellGrid{} }
func (f *fakePane) CaptureContent(history bool) ([]byte, error) { return nil, nil }
func (f *fakePane) Respawn(shell string) error                 { return nil }
func (f *fakePane) Close() error                               { f.closed = true; return nil }
func (f *fakePane) ShellPID() int                              { return 0 }
func (f *fakePane) LastOutputAt() time.Time                    { return time.Time{} }
func (f *fakePane) ConsumeBell() bool                          { return false }
func (f *fakePane) ClearHistory()                              {}
func (f *fakePane) ClearScreen() error                         { return nil }

// newTestMutatorWithPane creates a serverMutator backed by a fresh session.Server
// and a fake pane factory that returns fakePane instances.
func newTestMutatorWithPane() (*serverMutator, chan struct{}, *[]*fakePane) {
	state := session.NewServer()
	done := make(chan struct{})
	created := &[]*fakePane{}
	nextID := 0
	m := &serverMutator{
		state:    state,
		shutdown: func() { close(done) },
		newPane: func(cfg pane.Config) (session.Pane, error) {
			nextID++
			fp := &fakePane{id: cfg.ID}
			*created = append(*created, fp)
			return fp, nil
		},
	}
	return m, done, created
}

func TestNewWindow(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	wv, err := m.NewWindow(sv.ID, "mywin")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	if got := len(sess.Windows); got != 1 {
		t.Errorf("sess.Windows length = %d, want 1", got)
	}

	win := sess.Windows[0].Window
	if got := len(win.Panes); got != 1 {
		t.Errorf("win.Panes length = %d, want 1", got)
	}

	if wv.Name != "mywin" {
		t.Errorf("WindowView.Name = %q, want %q", wv.Name, "mywin")
	}
}

func TestNewWindowDefaultName(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	wv, err := m.NewWindow(sv.ID, "")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	if wv.Name == "" {
		t.Error("expected non-empty auto-generated window name, got empty string")
	}
}

func TestNewWindowSessionNotFound(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	if _, err := m.NewWindow("nonexistent", "w"); err == nil {
		t.Error("expected error for unknown session ID, got nil")
	}
}

func TestKillWindow(t *testing.T) {
	m, _, created := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv, err := m.NewWindow(sv.ID, "to-kill")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	if err := m.KillWindow(sv.ID, wv.ID); err != nil {
		t.Fatalf("KillWindow: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	if got := len(sess.Windows); got != 0 {
		t.Errorf("sess.Windows length after kill = %d, want 0", got)
	}

	if len(*created) == 0 {
		t.Fatal("no panes were created")
	}
	if !(*created)[0].closed {
		t.Error("fakePane.closed = false after KillWindow, want true")
	}
}

func TestKillWindowNotFound(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if err := m.KillWindow(sv.ID, "nonexistent"); err == nil {
		t.Error("expected error for unknown window ID, got nil")
	}
}

func TestRenameWindow(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv, err := m.NewWindow(sv.ID, "original")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	if err := m.RenameWindow(sv.ID, wv.ID, "renamed"); err != nil {
		t.Fatalf("RenameWindow: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	win := sess.Windows[0].Window
	if win.Name != "renamed" {
		t.Errorf("win.Name = %q, want %q", win.Name, "renamed")
	}
}

func TestSelectWindow(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv1, err := m.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow win1: %v", err)
	}
	wv2, err := m.NewWindow(sv.ID, "win2")
	if err != nil {
		t.Fatalf("NewWindow win2: %v", err)
	}

	if err := m.SelectWindow(sv.ID, wv2.ID); err != nil {
		t.Fatalf("SelectWindow: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	if sess.Current == nil {
		t.Fatal("sess.Current is nil after SelectWindow")
	}
	if string(sess.Current.Window.ID) != wv2.ID {
		t.Errorf("sess.Current.Window.ID = %q, want %q", sess.Current.Window.ID, wv2.ID)
	}
	_ = wv1
}

func TestSplitWindow(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv, err := m.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	pv, err := m.SplitWindow(sv.ID, wv.ID)
	if err != nil {
		t.Fatalf("SplitWindow: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	win := sess.Windows[0].Window
	if got := len(win.Panes); got != 2 {
		t.Errorf("win.Panes length = %d, want 2", got)
	}

	// Verify layout has two leaves.
	leafCount := 0
	for range win.Layout.Leaves() {
		leafCount++
	}
	if leafCount != 2 {
		t.Errorf("layout leaf count = %d, want 2", leafCount)
	}

	if pv.ID == 0 {
		t.Error("PaneView.ID should be non-zero after SplitWindow")
	}
}

func TestKillPane(t *testing.T) {
	m, _, created := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv, err := m.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	// Split to get a second pane so we can kill one without killing the window.
	pv, err := m.SplitWindow(sv.ID, wv.ID)
	if err != nil {
		t.Fatalf("SplitWindow: %v", err)
	}

	if err := m.KillPane(pv.ID); err != nil {
		t.Fatalf("KillPane: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	if len(sess.Windows) == 0 {
		t.Fatal("session has no windows after KillPane (expected window to survive)")
	}
	win := sess.Windows[0].Window
	if got := len(win.Panes); got != 1 {
		t.Errorf("win.Panes length = %d, want 1", got)
	}

	// The second (split) pane should have been closed.
	var splitPane *fakePane
	for _, fp := range *created {
		if int(fp.id) == pv.ID {
			splitPane = fp
			break
		}
	}
	if splitPane == nil {
		t.Fatal("could not find the split pane in created list")
	}
	if !splitPane.closed {
		t.Error("split fakePane.closed = false after KillPane, want true")
	}
}

func TestKillPaneKillsWindowWhenEmpty(t *testing.T) {
	m, _, created := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv, err := m.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	// Window has only one pane; killing it should remove the window too.
	paneID := wv.Active
	if err := m.KillPane(paneID); err != nil {
		t.Fatalf("KillPane: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	if got := len(sess.Windows); got != 0 {
		t.Errorf("sess.Windows length = %d, want 0 after killing last pane", got)
	}
	if len(*created) == 0 {
		t.Fatal("no panes were created")
	}
	if !(*created)[0].closed {
		t.Error("fakePane.closed = false after KillPane, want true")
	}
}

func TestSelectPane(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv, err := m.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	pv, err := m.SplitWindow(sv.ID, wv.ID)
	if err != nil {
		t.Fatalf("SplitWindow: %v", err)
	}

	if err := m.SelectPane(sv.ID, wv.ID, pv.ID); err != nil {
		t.Fatalf("SelectPane: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	win := sess.Windows[0].Window
	if int(win.Active) != pv.ID {
		t.Errorf("win.Active = %d, want %d", win.Active, pv.ID)
	}
}

// ─── MoveWindow tests ─────────────────────────────────────────────────────────

func TestMoveWindowAppendToEnd(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv1, err := m.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow win1: %v", err)
	}
	wv2, err := m.NewWindow(sv.ID, "win2")
	if err != nil {
		t.Fatalf("NewWindow win2: %v", err)
	}
	wv3, err := m.NewWindow(sv.ID, "win3")
	if err != nil {
		t.Fatalf("NewWindow win3: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	origIdx := sess.Windows[0].Index // index of win1

	// Move win1 (-1 = append to end).
	if err := m.MoveWindow(sv.ID, wv1.ID, -1); err != nil {
		t.Fatalf("MoveWindow: %v", err)
	}

	// win1 should now be last.
	last := sess.Windows[len(sess.Windows)-1]
	if last.Window.ID != session.WindowID(wv1.ID) {
		t.Errorf("last window = %q, want win1 (%q)", last.Window.ID, wv1.ID)
	}
	_ = origIdx
	_ = wv2
	_ = wv3
}

func TestMoveWindowToSpecificIndex(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv1, err := m.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow win1: %v", err)
	}
	wv2, err := m.NewWindow(sv.ID, "win2")
	if err != nil {
		t.Fatalf("NewWindow win2: %v", err)
	}
	wv3, err := m.NewWindow(sv.ID, "win3")
	if err != nil {
		t.Fatalf("NewWindow win3: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	// win3 is currently last; move it to index 1 (first position).
	win3Index := sess.Windows[2].Index

	if err := m.MoveWindow(sv.ID, wv3.ID, 1); err != nil {
		t.Fatalf("MoveWindow: %v", err)
	}

	first := sess.Windows[0]
	if first.Window.ID != session.WindowID(wv3.ID) {
		t.Errorf("first window = %q, want win3 (%q)", first.Window.ID, wv3.ID)
	}
	if first.Index != 1 {
		t.Errorf("first window index = %d, want 1", first.Index)
	}
	_ = wv1
	_ = wv2
	_ = win3Index
}

func TestMoveWindowNoOp(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv1, err := m.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow win1: %v", err)
	}
	wv2, err := m.NewWindow(sv.ID, "win2")
	if err != nil {
		t.Fatalf("NewWindow win2: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	before0 := sess.Windows[0].Window.ID
	before1 := sess.Windows[1].Window.ID

	// newIndex=0 should be a no-op.
	if err := m.MoveWindow(sv.ID, wv1.ID, 0); err != nil {
		t.Fatalf("MoveWindow: %v", err)
	}

	if sess.Windows[0].Window.ID != before0 || sess.Windows[1].Window.ID != before1 {
		t.Error("MoveWindow(newIndex=0) changed window order, expected no-op")
	}
	_ = wv2
}

func TestMoveWindowSessionNotFound(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	if err := m.MoveWindow("nonexistent", "w1", -1); err == nil {
		t.Error("expected error for unknown session, got nil")
	}
}

func TestMoveWindowNotFound(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if err := m.MoveWindow(sv.ID, "nonexistent", -1); err == nil {
		t.Error("expected error for unknown window, got nil")
	}
}

// ─── SwapWindows tests ────────────────────────────────────────────────────────

func TestSwapWindowsSameSession(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv1, err := m.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow win1: %v", err)
	}
	wv2, err := m.NewWindow(sv.ID, "win2")
	if err != nil {
		t.Fatalf("NewWindow win2: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	idx1Before := sess.Windows[0].Index
	idx2Before := sess.Windows[1].Index

	if err := m.SwapWindows(sv.ID, wv1.ID, wv2.ID); err != nil {
		t.Fatalf("SwapWindows: %v", err)
	}

	// After swap: win2 should have the original index of win1, and vice versa.
	var wl1, wl2 *session.Winlink
	for _, wl := range sess.Windows {
		switch wl.Window.ID {
		case session.WindowID(wv1.ID):
			wl1 = wl
		case session.WindowID(wv2.ID):
			wl2 = wl
		}
	}
	if wl1.Index != idx2Before {
		t.Errorf("win1 index after swap = %d, want %d", wl1.Index, idx2Before)
	}
	if wl2.Index != idx1Before {
		t.Errorf("win2 index after swap = %d, want %d", wl2.Index, idx1Before)
	}
	// The slice should be sorted: win2 is now first.
	if sess.Windows[0].Window.ID != session.WindowID(wv2.ID) {
		t.Errorf("first window after swap = %q, want win2 (%q)", sess.Windows[0].Window.ID, wv2.ID)
	}
}

func TestSwapWindowsThreeWindows(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv1, err := m.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow win1: %v", err)
	}
	wv2, err := m.NewWindow(sv.ID, "win2")
	if err != nil {
		t.Fatalf("NewWindow win2: %v", err)
	}
	wv3, err := m.NewWindow(sv.ID, "win3")
	if err != nil {
		t.Fatalf("NewWindow win3: %v", err)
	}

	// Swap win1 and win3; win2 stays in the middle.
	if err := m.SwapWindows(sv.ID, wv1.ID, wv3.ID); err != nil {
		t.Fatalf("SwapWindows: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	// win3 should now be first, win2 in the middle, win1 last.
	if sess.Windows[0].Window.ID != session.WindowID(wv3.ID) {
		t.Errorf("first window = %q, want win3", sess.Windows[0].Window.ID)
	}
	if sess.Windows[1].Window.ID != session.WindowID(wv2.ID) {
		t.Errorf("middle window = %q, want win2", sess.Windows[1].Window.ID)
	}
	if sess.Windows[2].Window.ID != session.WindowID(wv1.ID) {
		t.Errorf("last window = %q, want win1", sess.Windows[2].Window.ID)
	}
}

func TestSwapWindowsSessionNotFound(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	if err := m.SwapWindows("nonexistent", "w1", "w2"); err == nil {
		t.Error("expected error for unknown session, got nil")
	}
}

func TestSwapWindowsWindowNotFound(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv1, err := m.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	if err := m.SwapWindows(sv.ID, wv1.ID, "nonexistent"); err == nil {
		t.Error("expected error for unknown window B, got nil")
	}
}

func TestFindWindowByName(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv, err := m.NewWindow(sv.ID, "mywindow")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	got, err := m.FindWindow(sv.ID, "mywin")
	if err != nil {
		t.Fatalf("FindWindow: %v", err)
	}
	if got.ID != wv.ID {
		t.Errorf("FindWindow returned ID %q, want %q", got.ID, wv.ID)
	}
}

func TestFindWindowNoMatch(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := m.NewWindow(sv.ID, "alpha"); err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	if _, err := m.FindWindow(sv.ID, "beta"); err == nil {
		t.Error("expected error for no match, got nil")
	}
}

func TestFindWindowMultipleMatchesReturnsFirst(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv1, err := m.NewWindow(sv.ID, "work-alpha")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	if _, err := m.NewWindow(sv.ID, "work-beta"); err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	got, err := m.FindWindow(sv.ID, "work")
	if err != nil {
		t.Fatalf("FindWindow: %v", err)
	}
	if got.ID != wv1.ID {
		t.Errorf("FindWindow returned ID %q, want first match %q", got.ID, wv1.ID)
	}
}

func TestFindWindowSessionNotFound(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	if _, err := m.FindWindow("nonexistent", "anything"); err == nil {
		t.Error("expected error for unknown session, got nil")
	}
}

// ─── BreakPane tests ──────────────────────────────────────────────────────────

func TestBreakPane_CreatesNewWindow(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv, err := m.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	// Split to get two panes, then break one out.
	pv, err := m.SplitWindow(sv.ID, wv.ID)
	if err != nil {
		t.Fatalf("SplitWindow: %v", err)
	}

	newWv, err := m.BreakPane(sv.ID, wv.ID, pv.ID)
	if err != nil {
		t.Fatalf("BreakPane: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	if got := len(sess.Windows); got != 2 {
		t.Errorf("sess.Windows count = %d, want 2", got)
	}

	// The broken-out window should have exactly one pane.
	if got := len(newWv.Panes); got != 1 {
		t.Errorf("new WindowView pane count = %d, want 1", got)
	}

	// The original window should still have one pane.
	var origWin *session.Window
	for _, wl := range sess.Windows {
		if wl.Window.ID == session.WindowID(wv.ID) {
			origWin = wl.Window
			break
		}
	}
	if origWin == nil {
		t.Fatal("original window not found in session")
	}
	if got := len(origWin.Panes); got != 1 {
		t.Errorf("original window pane count = %d, want 1", got)
	}
}

func TestBreakPane_SourceWindowKilledWhenEmpty(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv, err := m.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	// Break the only pane out — source window should be destroyed.
	activePaneID := wv.Active
	_, err = m.BreakPane(sv.ID, wv.ID, activePaneID)
	if err != nil {
		t.Fatalf("BreakPane: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	// The original window is gone; only the new window remains.
	if got := len(sess.Windows); got != 1 {
		t.Errorf("sess.Windows count = %d, want 1 (new window only)", got)
	}
	// The remaining window must not be the original.
	if sess.Windows[0].Window.ID == session.WindowID(wv.ID) {
		t.Error("original window still present after breaking its only pane")
	}
}

func TestBreakPane_PaneNotFound(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv, err := m.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	if _, err := m.BreakPane(sv.ID, wv.ID, 9999); err == nil {
		t.Error("expected error for non-existent pane, got nil")
	}
}

// ─── JoinPane tests ───────────────────────────────────────────────────────────

func TestJoinPane_MovesPaneBetweenWindows(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv1, err := m.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow win1: %v", err)
	}
	wv2, err := m.NewWindow(sv.ID, "win2")
	if err != nil {
		t.Fatalf("NewWindow win2: %v", err)
	}

	// Split win1 so it has two panes, then join one into win2.
	pv, err := m.SplitWindow(sv.ID, wv1.ID)
	if err != nil {
		t.Fatalf("SplitWindow: %v", err)
	}

	if err := m.JoinPane(sv.ID, wv1.ID, pv.ID, sv.ID, wv2.ID); err != nil {
		t.Fatalf("JoinPane: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]

	var win1, win2 *session.Window
	for _, wl := range sess.Windows {
		switch wl.Window.ID {
		case session.WindowID(wv1.ID):
			win1 = wl.Window
		case session.WindowID(wv2.ID):
			win2 = wl.Window
		}
	}

	if win1 == nil || win2 == nil {
		t.Fatal("could not find both windows after JoinPane")
	}
	if got := len(win1.Panes); got != 1 {
		t.Errorf("win1 pane count = %d, want 1", got)
	}
	if got := len(win2.Panes); got != 2 {
		t.Errorf("win2 pane count = %d, want 2", got)
	}
}

func TestJoinPane_SourceWindowKilledWhenEmpty(t *testing.T) {
	m, _, _ := newTestMutatorWithPane()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wv1, err := m.NewWindow(sv.ID, "win1")
	if err != nil {
		t.Fatalf("NewWindow win1: %v", err)
	}
	wv2, err := m.NewWindow(sv.ID, "win2")
	if err != nil {
		t.Fatalf("NewWindow win2: %v", err)
	}

	// win1 has only one pane; joining it into win2 should destroy win1.
	activePaneID := wv1.Active
	if err := m.JoinPane(sv.ID, wv1.ID, activePaneID, sv.ID, wv2.ID); err != nil {
		t.Fatalf("JoinPane: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	if got := len(sess.Windows); got != 1 {
		t.Errorf("sess.Windows count = %d, want 1 (win1 should be destroyed)", got)
	}
	if sess.Windows[0].Window.ID != session.WindowID(wv2.ID) {
		t.Errorf("remaining window = %q, want win2 (%q)", sess.Windows[0].Window.ID, wv2.ID)
	}
}
