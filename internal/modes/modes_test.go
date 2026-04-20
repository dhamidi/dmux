package modes_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
)

// stubCanvas is a minimal Canvas implementation for testing.
type stubCanvas struct {
	rows, cols int
	cells      []modes.Cell
}

func newStubCanvas(cols, rows int) *stubCanvas {
	return &stubCanvas{
		rows:  rows,
		cols:  cols,
		cells: make([]modes.Cell, cols*rows),
	}
}

func (c *stubCanvas) Size() modes.Size { return modes.Size{Rows: c.rows, Cols: c.cols} }

func (c *stubCanvas) Set(col, row int, cell modes.Cell) {
	if col < 0 || col >= c.cols || row < 0 || row >= c.rows {
		return
	}
	c.cells[row*c.cols+col] = cell
}

// stubPaneMode is a minimal PaneMode stub that compiles without any
// other internal package dependencies.
type stubPaneMode struct{ closed bool }

func (m *stubPaneMode) Render(dst modes.Canvas) {
	sz := dst.Size()
	for row := 0; row < sz.Rows; row++ {
		for col := 0; col < sz.Cols; col++ {
			dst.Set(col, row, modes.Cell{Char: '.'})
		}
	}
}

func (m *stubPaneMode) Key(k keys.Key) modes.Outcome   { return modes.Consumed() }
func (m *stubPaneMode) Mouse(_ keys.MouseEvent) modes.Outcome { return modes.Passthrough() }
func (m *stubPaneMode) Close()                         { m.closed = true }

// Compile-time assertion: stubPaneMode satisfies PaneMode.
var _ modes.PaneMode = (*stubPaneMode)(nil)

// stubOverlay is a minimal ClientOverlay stub.
type stubOverlay struct {
	rect   modes.Rect
	closed bool
}

func (o *stubOverlay) Rect() modes.Rect                   { return o.rect }
func (o *stubOverlay) Render(dst []modes.Cell)            { /* fill with spaces */ }
func (o *stubOverlay) Key(k keys.Key) modes.Outcome       { return modes.CloseMode() }
func (o *stubOverlay) Mouse(_ keys.MouseEvent) modes.Outcome { return modes.Passthrough() }
func (o *stubOverlay) CaptureFocus() bool                 { return true }
func (o *stubOverlay) Close()                             { o.closed = true }

// Compile-time assertion: stubOverlay satisfies ClientOverlay.
var _ modes.ClientOverlay = (*stubOverlay)(nil)

func TestPaneModeStub(t *testing.T) {
	m := &stubPaneMode{}
	canvas := newStubCanvas(10, 5)

	m.Render(canvas)
	if canvas.cells[0].Char != '.' {
		t.Errorf("expected '.' at (0,0), got %q", canvas.cells[0].Char)
	}

	if out := m.Key(keys.Key{}); out.Kind != modes.KindConsumed {
		t.Errorf("Key: expected KindConsumed, got %v", out.Kind)
	}

	if out := m.Mouse(keys.MouseEvent{}); out.Kind != modes.KindPassthrough {
		t.Errorf("Mouse: expected KindPassthrough, got %v", out.Kind)
	}

	m.Close()
	if !m.closed {
		t.Error("Close did not set closed flag")
	}
}

func TestClientOverlayStub(t *testing.T) {
	o := &stubOverlay{rect: modes.Rect{X: 0, Y: 0, Width: 4, Height: 3}}

	if r := o.Rect(); r.Width != 4 || r.Height != 3 {
		t.Errorf("Rect: got %+v", r)
	}

	dst := make([]modes.Cell, o.rect.Width*o.rect.Height)
	o.Render(dst) // must not panic

	if out := o.Key(keys.Key{}); out.Kind != modes.KindCloseMode {
		t.Errorf("Key: expected KindCloseMode, got %v", out.Kind)
	}

	if out := o.Mouse(keys.MouseEvent{}); out.Kind != modes.KindPassthrough {
		t.Errorf("Mouse: expected KindPassthrough, got %v", out.Kind)
	}

	if !o.CaptureFocus() {
		t.Error("CaptureFocus should return true")
	}

	o.Close()
	if !o.closed {
		t.Error("Close did not set closed flag")
	}
}

func TestOutcomeConstructors(t *testing.T) {
	cases := []struct {
		name string
		got  modes.Outcome
		want modes.OutcomeKind
	}{
		{"Consumed", modes.Consumed(), modes.KindConsumed},
		{"Passthrough", modes.Passthrough(), modes.KindPassthrough},
		{"CloseMode", modes.CloseMode(), modes.KindCloseMode},
		{"Command", modes.Command("do-something"), modes.KindCommand},
	}
	for _, tc := range cases {
		if tc.got.Kind != tc.want {
			t.Errorf("%s: got Kind=%v, want %v", tc.name, tc.got.Kind, tc.want)
		}
	}
	cmd := modes.Command("action")
	if cmd.Cmd != "action" {
		t.Errorf("Command.Cmd: got %v, want %q", cmd.Cmd, "action")
	}
}
