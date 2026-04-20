package clock

import (
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/modes"
)

// stubCanvas is a simple Canvas implementation for testing.
type stubCanvas struct {
	cols, rows int
	cells      []modes.Cell
}

func newStubCanvas(cols, rows int) *stubCanvas {
	return &stubCanvas{
		cols:  cols,
		rows:  rows,
		cells: make([]modes.Cell, cols*rows),
	}
}

func (c *stubCanvas) Size() modes.Size { return modes.Size{Cols: c.cols, Rows: c.rows} }

func (c *stubCanvas) Set(col, row int, cell modes.Cell) {
	if col < 0 || col >= c.cols || row < 0 || row >= c.rows {
		return
	}
	c.cells[row*c.cols+col] = cell
}

// nonEmpty returns true if at least one cell has a non-zero Char.
func (c *stubCanvas) nonEmpty() bool {
	for _, cell := range c.cells {
		if cell.Char != 0 {
			return true
		}
	}
	return false
}

func TestMode_OverlayName(t *testing.T) {
	m := New(nil)
	if got := m.OverlayName(); got != "clock-mode" {
		t.Errorf("OverlayName() = %q, want %q", got, "clock-mode")
	}
}

func TestMode_Render_NonEmpty(t *testing.T) {
	// Fix time to 12:34 for deterministic output.
	fixed := time.Date(2024, 1, 1, 12, 34, 0, 0, time.UTC)
	m := New(func() time.Time { return fixed })

	canvas := newStubCanvas(80, 24)
	m.Render(canvas)

	if !canvas.nonEmpty() {
		t.Error("Render produced empty canvas for 80×24 grid")
	}
}

func TestMode_Render_SmallCanvas(t *testing.T) {
	// Even on a very small canvas, Render should not panic.
	fixed := time.Date(2024, 1, 1, 9, 5, 0, 0, time.UTC)
	m := New(func() time.Time { return fixed })

	canvas := newStubCanvas(5, 3)
	// Should not panic.
	m.Render(canvas)
}
