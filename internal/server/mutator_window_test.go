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
