package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/modes"
	popupmode "github.com/dhamidi/dmux/internal/modes/popup"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/session"
)

// newTestMutatorWithDisplayPopup creates a minimal serverMutator suitable for
// display-popup tests. It registers client "c1" with a 24×80 terminal and
// wires overlay push/pop callbacks for inspection.
func newTestMutatorWithDisplayPopup() (
	m *serverMutator,
	client *session.Client,
	pushed func() modes.ClientOverlay,
	popped func() bool,
) {
	state := session.NewServer()

	c := session.NewClient(session.ClientID("c1"))
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

	return m, c,
		func() modes.ClientOverlay { return lastOverlay },
		func() bool { return popCalled }
}

// TestDisplayPopup_AttachesOverlay verifies that DisplayPopup pushes a
// *popup.Mode as a ClientOverlay onto the target client.
func TestDisplayPopup_AttachesOverlay(t *testing.T) {
	m, _, pushed, _ := newTestMutatorWithDisplayPopup()

	if err := m.DisplayPopup("c1", "", "", 20, 10); err != nil {
		t.Fatalf("DisplayPopup: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("pushOverlayFn not called or overlay is nil")
	}
	if _, ok := ov.(*popupmode.Mode); !ok {
		t.Errorf("pushed overlay type = %T, want *popup.Mode", ov)
	}
}

// TestDisplayPopup_RectDimensions verifies that the popup overlay rect matches
// the requested width and height.
func TestDisplayPopup_RectDimensions(t *testing.T) {
	m, _, pushed, _ := newTestMutatorWithDisplayPopup()

	wantW, wantH := 40, 12
	if err := m.DisplayPopup("c1", "", "", wantW, wantH); err != nil {
		t.Fatalf("DisplayPopup: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("overlay is nil")
	}
	rect := ov.Rect()
	if rect.Width != wantW || rect.Height != wantH {
		t.Errorf("overlay rect = {%d×%d}, want {%d×%d}", rect.Width, rect.Height, wantW, wantH)
	}
}

// TestDisplayPopup_RectIsCentered verifies that the popup is centered within
// the client's terminal when no explicit position is given.
func TestDisplayPopup_RectIsCentered(t *testing.T) {
	m, _, pushed, _ := newTestMutatorWithDisplayPopup()

	// Client is 24 rows × 80 cols. Popup is 12×40.
	if err := m.DisplayPopup("c1", "", "", 40, 12); err != nil {
		t.Fatalf("DisplayPopup: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("overlay is nil")
	}
	rect := ov.Rect()
	wantX := (80 - 40) / 2 // 20
	wantY := (24 - 12) / 2 // 6
	if rect.X != wantX || rect.Y != wantY {
		t.Errorf("overlay position = (%d,%d), want (%d,%d)", rect.X, rect.Y, wantX, wantY)
	}
}

// TestDisplayPopup_ClientNotFound verifies that an error is returned when the
// client does not exist.
func TestDisplayPopup_ClientNotFound(t *testing.T) {
	m, _, _, _ := newTestMutatorWithDisplayPopup()

	if err := m.DisplayPopup("no-such-client", "", "", 20, 10); err == nil {
		t.Fatal("expected error for unknown client, got nil")
	}
}

// TestDisplayPopup_ClampsToClientSize verifies that requested dimensions
// larger than the client terminal are clamped.
func TestDisplayPopup_ClampsToClientSize(t *testing.T) {
	m, _, pushed, _ := newTestMutatorWithDisplayPopup()

	// Request dimensions larger than 24×80.
	if err := m.DisplayPopup("c1", "", "", 200, 100); err != nil {
		t.Fatalf("DisplayPopup: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("overlay is nil")
	}
	rect := ov.Rect()
	if rect.Width > 80 || rect.Height > 24 {
		t.Errorf("overlay rect = {%d×%d} exceeds client size {80×24}", rect.Width, rect.Height)
	}
}

// TestDisplayPopup_Renders verifies that the popup overlay renders without
// panic and produces the expected number of cells.
func TestDisplayPopup_Renders(t *testing.T) {
	m, _, pushed, _ := newTestMutatorWithDisplayPopup()

	w, h := 20, 10
	if err := m.DisplayPopup("c1", "", "", w, h); err != nil {
		t.Fatalf("DisplayPopup: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("overlay is nil")
	}

	dst := make([]modes.Cell, w*h)
	ov.Render(dst) // must not panic

	// The first cell should be a corner character '┌'.
	if dst[0].Char != '┌' {
		t.Errorf("dst[0].Char = %q, want '┌'", dst[0].Char)
	}
}

// TestDisplayPopup_WithCommandUsesPaneFactory verifies that when a command is
// provided, the newPane factory is invoked.
func TestDisplayPopup_WithCommandUsesPaneFactory(t *testing.T) {
	state := session.NewServer()

	c := session.NewClient(session.ClientID("c1"))
	c.Size = session.Size{Rows: 24, Cols: 80}
	state.Clients[c.ID] = c

	var lastOverlay modes.ClientOverlay
	factoryCalled := false

	m := &serverMutator{
		state:    state,
		shutdown: func() {},
		pushOverlayFn: func(_ session.ClientID, ov modes.ClientOverlay) {
			lastOverlay = ov
		},
		newPane: func(cfg pane.Config) (session.Pane, error) {
			factoryCalled = true
			return &fakePane{id: cfg.ID}, nil
		},
	}

	if err := m.DisplayPopup("c1", "echo hello", "", 20, 10); err != nil {
		t.Fatalf("DisplayPopup: %v", err)
	}

	if !factoryCalled {
		t.Error("newPane factory was not called when a command was provided")
	}
	if lastOverlay == nil {
		t.Fatal("overlay is nil")
	}
}
