package displaypanes

import (
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
)

const glyphW, glyphH = 3, 5

// numerals maps digit 0–9 to a 3×5 bitmap (row-major, 3 cols × 5 rows).
// '#' = filled cell, ' ' = empty cell.
var numerals = [10]string{
	"###" + "# #" + "# #" + "# #" + "###", // 0
	" # " + "## " + " # " + " # " + "###", // 1
	"###" + "  #" + "###" + "#  " + "###", // 2
	"###" + "  #" + "###" + "  #" + "###", // 3
	"# #" + "# #" + "###" + "  #" + "  #", // 4
	"###" + "#  " + "###" + "  #" + "###", // 5
	"###" + "#  " + "###" + "# #" + "###", // 6
	"###" + "  #" + "  #" + "  #" + "  #", // 7
	"###" + "# #" + "###" + "# #" + "###", // 8
	"###" + "# #" + "###" + "  #" + "###", // 9
}

// fillChar is the character used to render the filled cells of each numeral.
const fillChar = '█'

// PaneInfo describes a single pane for display-panes rendering.
type PaneInfo struct {
	ID     string     // pane identifier (opaque to this package)
	Number int        // displayed numeral (0–9)
	Bounds modes.Rect // position in client (screen) coordinates
}

// SelectPaneCommand is returned as a [modes.Command] outcome when the
// user presses a digit key that matches a pane's Number field.
type SelectPaneCommand struct {
	PaneNumber int
}

// Mode implements [modes.ClientOverlay] for display-panes.
//
// It draws big box-drawing numerals over each visible pane and waits
// for a digit key to select one. The host is responsible for scheduling
// the auto-dismiss timer and calling the dismiss function it receives
// from [New]'s scheduleTimeout argument.
type Mode struct {
	bounds    modes.Rect
	panes     []PaneInfo
	dismissed bool
}

// New creates a display-panes overlay covering bounds.
//
// panes is the list of visible panes with their numbers and screen positions.
//
// scheduleTimeout is called once at construction time with a dismiss function.
// The host should invoke that dismiss function after display-panes-time ms to
// auto-dismiss the overlay. Pass nil to disable auto-dismiss (useful in tests).
func New(bounds modes.Rect, panes []PaneInfo, scheduleTimeout func(dismiss func())) *Mode {
	m := &Mode{bounds: bounds, panes: panes}
	if scheduleTimeout != nil {
		scheduleTimeout(func() { m.dismissed = true })
	}
	return m
}

// Rect returns the overlay's bounding rectangle in screen (client) coordinates.
// This is the full coverage area passed at construction.
func (m *Mode) Rect() modes.Rect { return m.bounds }

// Render draws the big numeral for each pane into dst.
// dst has length Rect().Width × Rect().Height (row-major order).
func (m *Mode) Render(dst []modes.Cell) {
	for _, p := range m.panes {
		m.drawNumeral(dst, p)
	}
}

// Key handles keyboard input.
//
// Digit keys (0–9) that match a pane's Number return
// [modes.Command]([SelectPaneCommand]{PaneNumber}). Escape returns
// [modes.CloseMode]. Unknown keys return [modes.Consumed].
func (m *Mode) Key(k keys.Key) modes.Outcome {
	if k.Code == keys.CodeEscape {
		return modes.CloseMode()
	}
	if k.Code >= keys.KeyCode('0') && k.Code <= keys.KeyCode('9') {
		digit := int(k.Code - keys.KeyCode('0'))
		for _, p := range m.panes {
			if p.Number == digit {
				return modes.Command(SelectPaneCommand{PaneNumber: digit})
			}
		}
		// Digit pressed but no pane with that number — dismiss.
		return modes.CloseMode()
	}
	return modes.Consumed()
}

// Mouse is a no-op; display-panes does not handle mouse events.
func (m *Mode) Mouse(_ keys.MouseEvent) modes.Outcome { return modes.Consumed() }

// CaptureFocus returns true so digit keys are delivered to this overlay
// rather than the focused pane.
func (m *Mode) CaptureFocus() bool { return true }

// Close is a no-op; the mode holds no external resources.
func (m *Mode) Close() {}

// Dismissed reports whether the timeout dismiss callback has been called.
// The host should remove this overlay from the client overlay stack when
// Dismissed returns true.
func (m *Mode) Dismissed() bool { return m.dismissed }

// drawNumeral renders pane p's big numeral centered within p.Bounds,
// writing filled cells into dst (indexed relative to m.bounds).
func (m *Mode) drawNumeral(dst []modes.Cell, p PaneInfo) {
	if p.Number < 0 || p.Number > 9 {
		return
	}
	glyph := numerals[p.Number]
	startX := p.Bounds.X + (p.Bounds.Width-glyphW)/2
	startY := p.Bounds.Y + (p.Bounds.Height-glyphH)/2

	for row := 0; row < glyphH; row++ {
		for col := 0; col < glyphW; col++ {
			if glyph[row*glyphW+col] != '#' {
				continue
			}
			screenX := startX + col
			screenY := startY + row
			// Clip to overlay bounds.
			if screenX < m.bounds.X || screenX >= m.bounds.X+m.bounds.Width {
				continue
			}
			if screenY < m.bounds.Y || screenY >= m.bounds.Y+m.bounds.Height {
				continue
			}
			idx := (screenY-m.bounds.Y)*m.bounds.Width + (screenX - m.bounds.X)
			if idx >= 0 && idx < len(dst) {
				dst[idx] = modes.Cell{Char: fillChar}
			}
		}
	}
}
