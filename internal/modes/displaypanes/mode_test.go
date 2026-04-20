package displaypanes_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	dp "github.com/dhamidi/dmux/internal/modes/displaypanes"
)

// ---- interface compliance --------------------------------------------------

func TestImplementsClientOverlay(t *testing.T) {
	m := dp.New(modes.Rect{Width: 80, Height: 24}, nil, nil)
	var _ modes.ClientOverlay = m
}

// ---- helpers ---------------------------------------------------------------

func makeMode(panes []dp.PaneInfo) (*dp.Mode, func()) {
	bounds := modes.Rect{X: 0, Y: 0, Width: 80, Height: 24}
	var dismissFn func()
	m := dp.New(bounds, panes, func(d func()) { dismissFn = d })
	return m, dismissFn
}

// ---- rendering tests -------------------------------------------------------

func TestRender_DrawsNumeralCenteredInPane(t *testing.T) {
	// Single pane occupying the full overlay area.
	panes := []dp.PaneInfo{
		{ID: "p0", Number: 0, Bounds: modes.Rect{X: 0, Y: 0, Width: 20, Height: 10}},
	}
	m, _ := makeMode(panes)
	dst := make([]modes.Cell, 80*24)
	m.Render(dst)

	// At least one filled cell should be written for digit 0.
	found := false
	for _, c := range dst {
		if c.Char != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Render: expected at least one filled cell for digit 0")
	}
}

func TestRender_MultiplePanesDrawnInCorrectRegions(t *testing.T) {
	panes := []dp.PaneInfo{
		{ID: "p0", Number: 0, Bounds: modes.Rect{X: 0, Y: 0, Width: 40, Height: 24}},
		{ID: "p1", Number: 1, Bounds: modes.Rect{X: 40, Y: 0, Width: 40, Height: 24}},
	}
	m, _ := makeMode(panes)
	dst := make([]modes.Cell, 80*24)
	m.Render(dst)

	leftFilled, rightFilled := false, false
	for row := 0; row < 24; row++ {
		for col := 0; col < 40; col++ {
			if dst[row*80+col].Char != 0 {
				leftFilled = true
			}
		}
		for col := 40; col < 80; col++ {
			if dst[row*80+col].Char != 0 {
				rightFilled = true
			}
		}
	}
	if !leftFilled {
		t.Error("Render: expected filled cells in left pane (digit 0)")
	}
	if !rightFilled {
		t.Error("Render: expected filled cells in right pane (digit 1)")
	}
}

func TestRender_NoPanesProducesEmptyDst(t *testing.T) {
	m, _ := makeMode(nil)
	dst := make([]modes.Cell, 80*24)
	m.Render(dst)
	for i, c := range dst {
		if c.Char != 0 {
			t.Errorf("Render with no panes: dst[%d].Char = %q, want 0", i, c.Char)
		}
	}
}

func TestRender_GlyphCenteredInPane(t *testing.T) {
	// Use a small pane so we can verify centering precisely.
	// Glyph is 3 wide × 5 tall; pane is 7 wide × 7 tall.
	// Center offset: X = (7-3)/2 = 2, Y = (7-5)/2 = 1.
	panes := []dp.PaneInfo{
		{ID: "p1", Number: 1, Bounds: modes.Rect{X: 0, Y: 0, Width: 7, Height: 7}},
	}
	bounds := modes.Rect{X: 0, Y: 0, Width: 7, Height: 7}
	m := dp.New(bounds, panes, nil)
	dst := make([]modes.Cell, 7*7)
	m.Render(dst)

	// Digit 1 pattern: " # " / "## " / " # " / " # " / "###"
	// After centering (startX=2, startY=1), glyph col 1 (center col of glyph) maps to screen col 3.
	// Row 0 of glyph (screen row 1): col pattern " # " → col 1 of glyph = '#' → screen col 3.
	centerCol := 2 + 1 // startX + glyph column 1
	centerRow := 1 + 0 // startY + glyph row 0
	idx := centerRow*7 + centerCol
	if dst[idx].Char == 0 {
		t.Errorf("Render digit 1: expected filled cell at (%d,%d), got empty", centerCol, centerRow)
	}
}

// ---- key selection tests ---------------------------------------------------

func TestKey_DigitMatchesPaneReturnsCommand(t *testing.T) {
	panes := []dp.PaneInfo{
		{ID: "p1", Number: 1, Bounds: modes.Rect{X: 0, Y: 0, Width: 40, Height: 24}},
		{ID: "p2", Number: 2, Bounds: modes.Rect{X: 40, Y: 0, Width: 40, Height: 24}},
	}
	m, _ := makeMode(panes)

	out := m.Key(keys.Key{Code: keys.KeyCode('1')})
	if out.Kind != modes.KindCommand {
		t.Fatalf("Key('1'): want KindCommand, got %v", out.Kind)
	}
	cmd, ok := out.Cmd.(dp.SelectPaneCommand)
	if !ok {
		t.Fatalf("Key('1'): Cmd is %T, want SelectPaneCommand", out.Cmd)
	}
	if cmd.PaneNumber != 1 {
		t.Errorf("Key('1'): PaneNumber = %d, want 1", cmd.PaneNumber)
	}
}

func TestKey_DigitNoMatchClosesMode(t *testing.T) {
	panes := []dp.PaneInfo{
		{ID: "p1", Number: 1, Bounds: modes.Rect{X: 0, Y: 0, Width: 80, Height: 24}},
	}
	m, _ := makeMode(panes)

	// Pressing '5' when no pane has Number=5.
	out := m.Key(keys.Key{Code: keys.KeyCode('5')})
	if out.Kind != modes.KindCloseMode {
		t.Errorf("Key('5') no match: want KindCloseMode, got %v", out.Kind)
	}
}

func TestKey_EscapeClosesMode(t *testing.T) {
	m, _ := makeMode(nil)
	out := m.Key(keys.Key{Code: keys.CodeEscape})
	if out.Kind != modes.KindCloseMode {
		t.Errorf("Escape: want KindCloseMode, got %v", out.Kind)
	}
}

func TestKey_UnknownKeyConsumed(t *testing.T) {
	m, _ := makeMode(nil)
	out := m.Key(keys.Key{Code: keys.KeyCode('x')})
	if out.Kind != modes.KindConsumed {
		t.Errorf("unknown key 'x': want KindConsumed, got %v", out.Kind)
	}
}

func TestKey_AllDigits0Through9(t *testing.T) {
	panes := make([]dp.PaneInfo, 10)
	for i := range panes {
		panes[i] = dp.PaneInfo{
			ID:     "p",
			Number: i,
			Bounds: modes.Rect{X: i * 8, Y: 0, Width: 8, Height: 24},
		}
	}
	m, _ := makeMode(panes)

	for digit := 0; digit <= 9; digit++ {
		out := m.Key(keys.Key{Code: keys.KeyCode(rune('0' + digit))})
		if out.Kind != modes.KindCommand {
			t.Errorf("Key('%d'): want KindCommand, got %v", digit, out.Kind)
			continue
		}
		cmd, ok := out.Cmd.(dp.SelectPaneCommand)
		if !ok {
			t.Errorf("Key('%d'): Cmd is %T, want SelectPaneCommand", digit, out.Cmd)
			continue
		}
		if cmd.PaneNumber != digit {
			t.Errorf("Key('%d'): PaneNumber = %d, want %d", digit, cmd.PaneNumber, digit)
		}
	}
}

// ---- timeout tests ---------------------------------------------------------

func TestTimeout_NotDismissedInitially(t *testing.T) {
	m, _ := makeMode(nil)
	if m.Dismissed() {
		t.Error("Dismissed() should be false before timeout fires")
	}
}

func TestTimeout_DismissedAfterCallbackFires(t *testing.T) {
	m, dismiss := makeMode(nil)
	dismiss()
	if !m.Dismissed() {
		t.Error("Dismissed() should be true after dismiss callback is called")
	}
}

func TestTimeout_NilSchedulerDoesNotPanic(t *testing.T) {
	m := dp.New(modes.Rect{Width: 80, Height: 24}, nil, nil)
	if m.Dismissed() {
		t.Error("Dismissed() should be false with nil scheduler")
	}
}

// ---- CaptureFocus ----------------------------------------------------------

func TestCaptureFocus_ReturnsTrue(t *testing.T) {
	m, _ := makeMode(nil)
	if !m.CaptureFocus() {
		t.Error("CaptureFocus() should return true")
	}
}

// ---- Mouse -----------------------------------------------------------------

func TestMouse_ReturnsConsumed(t *testing.T) {
	m, _ := makeMode(nil)
	out := m.Mouse(keys.MouseEvent{})
	if out.Kind != modes.KindConsumed {
		t.Errorf("Mouse: want KindConsumed, got %v", out.Kind)
	}
}

// ---- Rect ------------------------------------------------------------------

func TestRect_ReturnsConstructorBounds(t *testing.T) {
	want := modes.Rect{X: 5, Y: 3, Width: 70, Height: 20}
	m := dp.New(want, nil, nil)
	if got := m.Rect(); got != want {
		t.Errorf("Rect() = %v, want %v", got, want)
	}
}
