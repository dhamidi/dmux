package server

import (
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/session"
)

// resizingPane is a session.Pane that records every Resize call.
type resizingPane struct {
	id      session.PaneID
	resizes []struct{ cols, rows int }
}

func (r *resizingPane) Title() string                                { return "resizing" }
func (r *resizingPane) Write(data []byte) error                      { return nil }
func (r *resizingPane) SendKey(key keys.Key) error                   { return nil }
func (r *resizingPane) Resize(cols, rows int) error {
	r.resizes = append(r.resizes, struct{ cols, rows int }{cols, rows})
	return nil
}
func (r *resizingPane) Snapshot() pane.CellGrid                     { return pane.CellGrid{} }
func (r *resizingPane) CaptureContent(history bool) ([]byte, error) { return nil, nil }
func (r *resizingPane) Respawn(shell string) error                   { return nil }
func (r *resizingPane) Close() error                                 { return nil }
func (r *resizingPane) ShellPID() int                                { return 0 }
func (r *resizingPane) LastOutputAt() time.Time                      { return time.Time{} }
func (r *resizingPane) ConsumeBell() bool                            { return false }
func (r *resizingPane) ClearHistory()                                {}
func (r *resizingPane) ClearScreen() error                           { return nil }

// newTestMutatorForResize creates a serverMutator with a session containing a
// window split into two panes vertically (pane 1 on top, pane 2 on bottom).
// The window is 80×24; each pane starts at 80×12.
// Returns the mutator, a slice of created panes (index 0 = pane 1, 1 = pane 2),
// plus the IDs of both panes.
func newTestMutatorForResize(t *testing.T) (m *serverMutator, panes []*resizingPane, pane1ID, pane2ID int) {
	t.Helper()

	state := session.NewServer()
	var created []*resizingPane
	mut := &serverMutator{
		state:    state,
		shutdown: func() {},
		newPane: func(cfg pane.Config) (session.Pane, error) {
			rp := &resizingPane{id: cfg.ID}
			created = append(created, rp)
			return rp, nil
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

	// Capture the first pane ID from the live window state.
	sess := mut.state.Sessions[session.SessionID(sv.ID)]
	win := sess.Windows[0].Window
	var p1ID int
	for id := range win.Panes {
		p1ID = int(id)
		break
	}

	// Split to create a second pane below the first (Vertical split).
	pv, err := mut.SplitWindow(sv.ID, wv.ID)
	if err != nil {
		t.Fatalf("SplitWindow: %v", err)
	}

	// Clear the resize records captured during setup.
	for _, rp := range created {
		rp.resizes = nil
	}

	return mut, created, p1ID, pv.ID
}

// TestResizePane_RelativeDown verifies that resizing pane 1 downward makes it
// taller and keeps the total height intact.
func TestResizePane_RelativeDown(t *testing.T) {
	m, panes, pane1ID, _ := newTestMutatorForResize(t)

	sess := m.state.Sessions["s1"]
	win := sess.Windows[0].Window

	r1before := win.Layout.Rect(session.PaneID(pane1ID))
	if r1before.Height == 0 {
		t.Fatal("pane 1 has zero height before resize")
	}

	if err := m.ResizePane(pane1ID, "D", 3); err != nil {
		t.Fatalf("ResizePane: %v", err)
	}

	r1after := win.Layout.Rect(session.PaneID(pane1ID))
	r2after := win.Layout.Rect(session.PaneID(panes[1].id))

	if r1after.Height <= r1before.Height {
		t.Errorf("pane 1 should have grown taller: %d -> %d", r1before.Height, r1after.Height)
	}
	if got := r1after.Height + r2after.Height; got != 24 {
		t.Errorf("heights should still sum to 24, got %d (%d + %d)", got, r1after.Height, r2after.Height)
	}
}

// TestResizePane_RelativeUp verifies that resizing pane 2 upward makes pane 2
// taller (and pane 1 shorter).
func TestResizePane_RelativeUp(t *testing.T) {
	m, panes, _, pane2ID := newTestMutatorForResize(t)

	sess := m.state.Sessions["s1"]
	win := sess.Windows[0].Window

	r2before := win.Layout.Rect(session.PaneID(pane2ID))

	if err := m.ResizePane(pane2ID, "U", 3); err != nil {
		t.Fatalf("ResizePane: %v", err)
	}

	r1after := win.Layout.Rect(session.PaneID(panes[0].id))
	r2after := win.Layout.Rect(session.PaneID(pane2ID))

	if r2after.Height <= r2before.Height {
		t.Errorf("pane 2 should have grown taller: %d -> %d", r2before.Height, r2after.Height)
	}
	if got := r1after.Height + r2after.Height; got != 24 {
		t.Errorf("heights should still sum to 24, got %d", got)
	}
}

// TestResizePane_PTYNotification verifies that ResizePane calls Resize on all
// pane PTYs with updated dimensions.
func TestResizePane_PTYNotification(t *testing.T) {
	m, panes, pane1ID, _ := newTestMutatorForResize(t)

	if err := m.ResizePane(pane1ID, "D", 3); err != nil {
		t.Fatalf("ResizePane: %v", err)
	}

	// Both panes should have received a Resize call with non-zero dimensions.
	for i, rp := range panes {
		if len(rp.resizes) == 0 {
			t.Errorf("pane %d (index %d): expected at least one Resize call, got none", rp.id, i)
			continue
		}
		last := rp.resizes[len(rp.resizes)-1]
		if last.cols <= 0 || last.rows <= 0 {
			t.Errorf("pane %d: Resize called with non-positive dimensions cols=%d rows=%d", rp.id, last.cols, last.rows)
		}
	}
}

// TestResizePane_PaneNotFound verifies that an error is returned when the pane
// ID does not exist.
func TestResizePane_PaneNotFound(t *testing.T) {
	m, _, _, _ := newTestMutatorForResize(t)

	if err := m.ResizePane(9999, "D", 1); err == nil {
		t.Error("expected error for unknown pane ID, got nil")
	}
}

// TestResizePane_UnknownDirection verifies that an unrecognised direction string
// returns an error.
func TestResizePane_UnknownDirection(t *testing.T) {
	m, _, pane1ID, _ := newTestMutatorForResize(t)

	if err := m.ResizePane(pane1ID, "X", 1); err == nil {
		t.Error("expected error for unknown direction, got nil")
	}
}
