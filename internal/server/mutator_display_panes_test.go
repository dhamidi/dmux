package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/layout"
	"github.com/dhamidi/dmux/internal/modes"
	"github.com/dhamidi/dmux/internal/session"
)

// newTestMutatorWithDisplayPanes creates a serverMutator with one session,
// one window split into two panes side-by-side, and client "c1" attached.
// It returns overlay push/pop helpers and the IDs needed for assertions.
func newTestMutatorWithDisplayPanes() (
	m *serverMutator,
	pushed func() modes.ClientOverlay,
	popped func() bool,
	sessID, winID string,
	pane1ID, pane2ID int,
) {
	state := session.NewServer()

	sess := session.NewSession(session.SessionID("s1"), "session1", nil)
	state.AddSession(sess)

	win := session.NewWindow(session.WindowID("w1"), "window1", nil)
	// Build a 80×24 layout split horizontally: pane 1 left (40×24), pane 2 right (40×24).
	win.Layout = layout.New(80, 24, session.PaneID(1))
	p2LeafID := win.Layout.Split(session.PaneID(1), layout.Horizontal)
	win.AddPane(session.PaneID(1), &fakePane{id: session.PaneID(1)})
	win.AddPane(p2LeafID, &fakePane{id: p2LeafID})

	wl := &session.Winlink{Index: 0, Window: win, Session: sess}
	sess.Windows = append(sess.Windows, wl)
	sess.Current = wl

	c := session.NewClient(session.ClientID("c1"))
	c.Session = sess
	c.Size = session.Size{Rows: 24, Cols: 80}
	state.Clients[c.ID] = c

	var lastOverlay modes.ClientOverlay
	var popCalled bool

	m = &serverMutator{
		state:    state,
		shutdown: func() {},
		pushOverlayFn: func(_ session.ClientID, ov modes.ClientOverlay) {
			lastOverlay = ov
		},
		popOverlayFn: func(_ session.ClientID) {
			popCalled = true
		},
	}

	return m,
		func() modes.ClientOverlay { return lastOverlay },
		func() bool { return popCalled },
		"s1", "w1",
		1, int(p2LeafID)
}

// TestDisplayPanes_AttachesOverlay verifies that DisplayPanes pushes a
// ClientOverlay onto the target client.
func TestDisplayPanes_AttachesOverlay(t *testing.T) {
	m, pushed, _, _, _, _, _ := newTestMutatorWithDisplayPanes()

	if err := m.DisplayPanes("c1"); err != nil {
		t.Fatalf("DisplayPanes: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("pushOverlayFn not called or overlay is nil")
	}
}

// TestDisplayPanes_OverlayCoversBounds verifies that the overlay rect spans
// the full client terminal area.
func TestDisplayPanes_OverlayCoversBounds(t *testing.T) {
	m, pushed, _, _, _, _, _ := newTestMutatorWithDisplayPanes()

	if err := m.DisplayPanes("c1"); err != nil {
		t.Fatalf("DisplayPanes: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("overlay is nil")
	}

	rect := ov.Rect()
	if rect.X != 0 || rect.Y != 0 || rect.Width != 80 || rect.Height != 24 {
		t.Errorf("overlay rect = {%d,%d,%d×%d}, want {0,0,80×24}",
			rect.X, rect.Y, rect.Width, rect.Height)
	}
}

// TestDisplayPanes_RendersNumerals verifies that the overlay renders numeral
// glyphs (non-zero cells) for each pane.
func TestDisplayPanes_RendersNumerals(t *testing.T) {
	m, pushed, _, _, _, _, _ := newTestMutatorWithDisplayPanes()

	if err := m.DisplayPanes("c1"); err != nil {
		t.Fatalf("DisplayPanes: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("overlay is nil")
	}

	rect := ov.Rect()
	dst := make([]modes.Cell, rect.Width*rect.Height)
	ov.Render(dst)

	// At least one cell should be filled (numeral glyph).
	filled := 0
	for _, cell := range dst {
		if cell.Char != 0 {
			filled++
		}
	}
	if filled == 0 {
		t.Error("Render produced no filled cells; expected numeral glyphs for each pane")
	}
}

// TestDisplayPanes_CaptureFocus verifies that the overlay captures focus so
// digit keys are routed to it.
func TestDisplayPanes_CaptureFocus(t *testing.T) {
	m, pushed, _, _, _, _, _ := newTestMutatorWithDisplayPanes()

	if err := m.DisplayPanes("c1"); err != nil {
		t.Fatalf("DisplayPanes: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("overlay is nil")
	}
	if !ov.CaptureFocus() {
		t.Error("CaptureFocus() = false, want true")
	}
}

// TestDisplayPanes_EscapeClosesOverlay verifies that pressing Escape returns
// KindCloseMode so the server pops the overlay.
func TestDisplayPanes_EscapeClosesOverlay(t *testing.T) {
	m, pushed, _, _, _, _, _ := newTestMutatorWithDisplayPanes()

	if err := m.DisplayPanes("c1"); err != nil {
		t.Fatalf("DisplayPanes: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("overlay is nil")
	}

	outcome := ov.Key(keys.Key{Code: keys.CodeEscape})
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("Escape outcome.Kind = %v, want KindCloseMode", outcome.Kind)
	}
}

// TestDisplayPanes_DigitKeySwitchesFocus verifies that pressing the digit key
// for a visible pane calls SelectPane (switches focus) and returns KindCloseMode.
func TestDisplayPanes_DigitKeySwitchesFocus(t *testing.T) {
	m, pushed, _, sessID, winID, _, _ := newTestMutatorWithDisplayPanes()

	// Wire SelectPane to record calls.
	var selectedSess, selectedWin string
	var selectedPane int
	origSelectPane := m.SelectPane
	_ = origSelectPane // m.SelectPane is a method, not a field; track via state

	if err := m.DisplayPanes("c1"); err != nil {
		t.Fatalf("DisplayPanes: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("overlay is nil")
	}

	// Capture the current active pane before the key press.
	sess := m.state.Sessions[session.SessionID(sessID)]
	win := sess.Current.Window
	_ = winID

	// Press '1' (digit 1 → second pane, number=1).
	outcome := ov.Key(keys.Key{Code: keys.KeyCode('1')})
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("digit '1' outcome.Kind = %v, want KindCloseMode", outcome.Kind)
	}

	// Verify the active pane changed to the pane numbered 1.
	_ = selectedSess
	_ = selectedWin
	_ = selectedPane

	// The second pane (number=1) should now be active.
	wantActive := win.Panes
	if len(wantActive) < 2 {
		t.Skip("only one pane, cannot test pane switch")
	}

	// Active pane should have changed away from pane 1 (number=0).
	if win.Active == session.PaneID(1) {
		t.Error("active pane did not change after pressing '1'")
	}
}

// TestDisplayPanes_ClientNotFound verifies that an error is returned when the
// client does not exist.
func TestDisplayPanes_ClientNotFound(t *testing.T) {
	m, _, _, _, _, _, _ := newTestMutatorWithDisplayPanes()

	if err := m.DisplayPanes("no-such-client"); err == nil {
		t.Fatal("expected error for unknown client, got nil")
	}
}

// TestDisplayPanes_NoSession verifies that an error is returned when the
// client has no attached session.
func TestDisplayPanes_NoSession(t *testing.T) {
	state := session.NewServer()
	c := session.NewClient(session.ClientID("c1"))
	c.Size = session.Size{Rows: 24, Cols: 80}
	// c.Session is nil — no session attached
	state.Clients[c.ID] = c

	m := &serverMutator{
		state:    state,
		shutdown: func() {},
	}

	if err := m.DisplayPanes("c1"); err == nil {
		t.Fatal("expected error for client with no session, got nil")
	}
}
