// Package clock implements the clock-mode overlay, which displays a large
// ASCII-art clock (HH:MM) centred in the pane area. It implements
// [modes.PaneMode] and [session.Overlay].
package clock

import (
	"fmt"
	"time"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
)

const glyphW, glyphH = 3, 5

// digits maps 0–9 to a 3×5 bitmap (row-major, 3 cols × 5 rows).
// '#' = filled cell, ' ' = empty cell.
var digits = [10]string{
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

const fillChar = '█'

// colon is a 1×5 bitmap for the ':' separator.
var colon = [glyphH]bool{false, true, false, true, false}

// Mode implements modes.PaneMode and session.Overlay for clock-mode.
type Mode struct {
	now func() time.Time // injectable for testing
}

// New creates a new clock Mode. If nowFn is nil, time.Now is used.
func New(nowFn func() time.Time) *Mode {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &Mode{now: nowFn}
}

// OverlayName implements session.Overlay.
func (m *Mode) OverlayName() string { return "clock-mode" }

// Render draws the HH:MM clock centred in the canvas.
// It implements modes.PaneMode.Render.
func (m *Mode) Render(dst modes.Canvas) {
	t := m.now()
	h, min := t.Hour(), t.Minute()
	timeStr := fmt.Sprintf("%02d:%02d", h, min)

	// Glyph layout: D D : D D
	// Widths:       3 3 1 3 3 + 3 gaps between glyphs (each 1 wide)
	// Total width  = 3+1+3+1+1+1+3+1+3 = 17, total height = 5
	const (
		colonW = 1
		gap    = 1
		totalW = glyphW + gap + glyphW + gap + colonW + gap + glyphW + gap + glyphW
		totalH = glyphH
	)

	size := dst.Size()
	startCol := (size.Cols - totalW) / 2
	startRow := (size.Rows - totalH) / 2

	// Parse HH:MM into digit indices.
	d := [4]int{int(timeStr[0] - '0'), int(timeStr[1] - '0'), int(timeStr[3] - '0'), int(timeStr[4] - '0')}

	// x positions of the 4 digits and colon:
	xPositions := [5]int{
		startCol,
		startCol + glyphW + gap,
		startCol + glyphW + gap + glyphW + gap, // colon
		startCol + glyphW + gap + glyphW + gap + colonW + gap,
		startCol + glyphW + gap + glyphW + gap + colonW + gap + glyphW + gap,
	}

	for row := 0; row < glyphH; row++ {
		// Draw first two digits (hours).
		for di := 0; di < 2; di++ {
			bitmap := digits[d[di]]
			for col := 0; col < glyphW; col++ {
				idx := row*glyphW + col
				if idx < len(bitmap) && bitmap[idx] == '#' {
					dst.Set(xPositions[di]+col, startRow+row, modes.Cell{Char: fillChar})
				}
			}
		}
		// Draw colon.
		if colon[row] {
			dst.Set(xPositions[2], startRow+row, modes.Cell{Char: ':'})
		}
		// Draw last two digits (minutes).
		for di := 0; di < 2; di++ {
			bitmap := digits[d[2+di]]
			for col := 0; col < glyphW; col++ {
				idx := row*glyphW + col
				if idx < len(bitmap) && bitmap[idx] == '#' {
					dst.Set(xPositions[3+di]+col, startRow+row, modes.Cell{Char: fillChar})
				}
			}
		}
	}
}

// Key closes clock-mode on any key press.
func (m *Mode) Key(_ keys.Key) modes.Outcome { return modes.CloseMode() }

// Mouse is a no-op.
func (m *Mode) Mouse(_ keys.MouseEvent) modes.Outcome { return modes.Consumed() }

// Close is a no-op; clock-mode holds no external resources.
func (m *Mode) Close() {}
