package render

import (
	"bytes"
	"fmt"
)

// appendCellStyles writes the SGR attribute and color sequences for cell into
// buf. It does not emit a reset; callers should emit "\x1b[0m" first.
func appendCellStyles(buf *bytes.Buffer, cell Cell) {
	if cell.Attrs&AttrBold != 0 {
		buf.WriteString("\x1b[1m")
	}
	if cell.Attrs&AttrDim != 0 {
		buf.WriteString("\x1b[2m")
	}
	if cell.Attrs&AttrItalics != 0 {
		buf.WriteString("\x1b[3m")
	}
	if cell.Attrs&AttrUnderline != 0 {
		buf.WriteString("\x1b[4m")
	}
	if cell.Attrs&AttrBlink != 0 {
		buf.WriteString("\x1b[5m")
	}
	if cell.Attrs&AttrReverse != 0 {
		buf.WriteString("\x1b[7m")
	}
	if cell.Attrs&AttrStrikethrough != 0 {
		buf.WriteString("\x1b[9m")
	}
	if cell.Attrs&AttrDoubleUnderline != 0 {
		buf.WriteString("\x1b[21m")
	}
	if cell.Attrs&AttrOverline != 0 {
		buf.WriteString("\x1b[53m")
	}
	if cell.Attrs&AttrCurlyUnderline != 0 {
		buf.WriteString("\x1b[4:3m")
	}

	switch cell.Fg & 0xFF00 {
	case ColorIndexed:
		fmt.Fprintf(buf, "\x1b[38;5;%dm", uint8(cell.Fg))
	case ColorRGB:
		fmt.Fprintf(buf, "\x1b[38;2;%d;%d;%dm", cell.FgR, cell.FgG, cell.FgB)
	}

	switch cell.Bg & 0xFF00 {
	case ColorIndexed:
		fmt.Fprintf(buf, "\x1b[48;5;%dm", uint8(cell.Bg))
	case ColorRGB:
		fmt.Fprintf(buf, "\x1b[48;2;%d;%d;%dm", cell.BgR, cell.BgG, cell.BgB)
	}
}

// EncodeANSI encodes grid as a sequence of ANSI escape codes suitable for
// writing to a VT100-compatible terminal. It always emits a full repaint:
// cursor home, erase display, then character data row by row.
func EncodeANSI(grid CellGrid) []byte {
	var buf bytes.Buffer

	// Cursor home + erase display.
	buf.WriteString("\x1b[H\x1b[2J")

	for r := 0; r < grid.Rows; r++ {
		for c := 0; c < grid.Cols; c++ {
			cell := grid.Cells[r*grid.Cols+c]

			styled := cell.Fg != ColorDefault || cell.Bg != ColorDefault || cell.Attrs != 0
			if styled {
				buf.WriteString("\x1b[0m")
				appendCellStyles(&buf, cell)
			}

			ch := cell.Char
			if ch == 0 {
				ch = ' '
			}
			buf.WriteRune(ch)
		}

		if r < grid.Rows-1 {
			buf.WriteString("\r\n")
		}
	}

	buf.WriteString("\x1b[0m")
	return buf.Bytes()
}

// EncodeDiffANSI encodes only the cells that differ between prev and curr as
// ANSI cursor-position + character sequences. Each changed cell is preceded by
// a cursor-move ("\x1b[row;colH") and an SGR reset so the output is
// self-contained. The output is wrapped with cursor-hide/show to prevent
// visual artifacts during incremental updates.
//
// Callers must ensure prev and curr have identical dimensions; if they do not,
// or if prev is empty, callers should fall back to [EncodeANSI].
func EncodeDiffANSI(prev, curr CellGrid) []byte {
	var buf bytes.Buffer
	buf.WriteString("\x1b[?25l") // hide cursor during update

	for r := 0; r < curr.Rows; r++ {
		for c := 0; c < curr.Cols; c++ {
			idx := r*curr.Cols + c
			cell := curr.Cells[idx]
			if cell == prev.Cells[idx] {
				continue
			}
			// Cursor move to this cell (1-based row and column).
			fmt.Fprintf(&buf, "\x1b[%d;%dH", r+1, c+1)
			// Always reset SGR so each changed cell is self-contained.
			buf.WriteString("\x1b[0m")
			styled := cell.Fg != ColorDefault || cell.Bg != ColorDefault || cell.Attrs != 0
			if styled {
				appendCellStyles(&buf, cell)
			}
			ch := cell.Char
			if ch == 0 {
				ch = ' '
			}
			buf.WriteRune(ch)
		}
	}

	buf.WriteString("\x1b[0m")   // reset SGR attributes
	buf.WriteString("\x1b[?25h") // show cursor
	return buf.Bytes()
}
