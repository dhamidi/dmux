package render

import (
	"bytes"
	"fmt"
)

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

				if cell.Attrs&AttrBold != 0 {
					buf.WriteString("\x1b[1m")
				}
				if cell.Attrs&AttrDim != 0 {
					buf.WriteString("\x1b[2m")
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

				switch cell.Fg & 0xFF00 {
				case ColorIndexed:
					fmt.Fprintf(&buf, "\x1b[38;5;%dm", uint8(cell.Fg))
				case ColorRGB:
					fmt.Fprintf(&buf, "\x1b[38;2;%d;%d;%dm", cell.FgR, cell.FgG, cell.FgB)
				}

				switch cell.Bg & 0xFF00 {
				case ColorIndexed:
					fmt.Fprintf(&buf, "\x1b[48;5;%dm", uint8(cell.Bg))
				case ColorRGB:
					fmt.Fprintf(&buf, "\x1b[48;2;%d;%d;%dm", cell.BgR, cell.BgG, cell.BgB)
				}
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
