package render

import (
	"strings"
	"testing"
)

func TestEncodeANSI_CursorHomeAndChars(t *testing.T) {
	grid := CellGrid{
		Rows: 2, Cols: 2,
		Cells: []Cell{
			{Char: 'A'}, {Char: 'B'},
			{Char: 'C'}, {Char: 'D'},
		},
	}
	out := EncodeANSI(grid)
	s := string(out)
	if !strings.Contains(s, "\x1b[H") {
		t.Error("missing cursor home")
	}
	for _, ch := range []string{"A", "B", "C", "D"} {
		if !strings.Contains(s, ch) {
			t.Errorf("missing character %q", ch)
		}
	}
}

func TestEncodeANSI_BoldAttr(t *testing.T) {
	grid := CellGrid{
		Rows: 1, Cols: 1,
		Cells: []Cell{
			{Char: 'X', Attrs: AttrBold},
		},
	}
	out := EncodeANSI(grid)
	s := string(out)
	if !strings.Contains(s, "\x1b[1m") {
		t.Error("missing bold SGR sequence \\x1b[1m")
	}
	if !strings.Contains(s, "X") {
		t.Error("missing character X")
	}
}

func TestEncodeANSI_IndexedColor(t *testing.T) {
	grid := CellGrid{
		Rows: 1, Cols: 1,
		Cells: []Cell{
			{Char: 'Z', Fg: ColorIndexed | 42},
		},
	}
	out := EncodeANSI(grid)
	s := string(out)
	if !strings.Contains(s, "\x1b[38;5;42m") {
		t.Errorf("missing indexed foreground SGR; got %q", s)
	}
}

func TestEncodeANSI_RGBColor(t *testing.T) {
	grid := CellGrid{
		Rows: 1, Cols: 1,
		Cells: []Cell{
			{Char: 'W', Bg: ColorRGB, BgR: 10, BgG: 20, BgB: 30},
		},
	}
	out := EncodeANSI(grid)
	s := string(out)
	if !strings.Contains(s, "\x1b[48;2;10;20;30m") {
		t.Errorf("missing RGB background SGR; got %q", s)
	}
}

func TestEncodeANSI_RowSeparator(t *testing.T) {
	grid := CellGrid{
		Rows: 2, Cols: 1,
		Cells: []Cell{{Char: 'A'}, {Char: 'B'}},
	}
	out := EncodeANSI(grid)
	s := string(out)
	if !strings.Contains(s, "\r\n") {
		t.Error("missing row separator \\r\\n")
	}
}

func TestEncodeANSI_TrailingReset(t *testing.T) {
	grid := CellGrid{
		Rows: 1, Cols: 1,
		Cells: []Cell{{Char: 'A'}},
	}
	out := EncodeANSI(grid)
	s := string(out)
	if !strings.HasSuffix(s, "\x1b[0m") {
		t.Errorf("output does not end with attribute reset; got %q", s)
	}
}

func TestEncodeANSI_ItalicsAttr(t *testing.T) {
	grid := CellGrid{
		Rows: 1, Cols: 1,
		Cells: []Cell{{Char: 'X', Attrs: AttrItalics}},
	}
	s := string(EncodeANSI(grid))
	if !strings.Contains(s, "\x1b[3m") {
		t.Errorf("missing italics SGR \\x1b[3m; got %q", s)
	}
}

func TestEncodeANSI_StrikethroughAttr(t *testing.T) {
	grid := CellGrid{
		Rows: 1, Cols: 1,
		Cells: []Cell{{Char: 'X', Attrs: AttrStrikethrough}},
	}
	s := string(EncodeANSI(grid))
	if !strings.Contains(s, "\x1b[9m") {
		t.Errorf("missing strikethrough SGR \\x1b[9m; got %q", s)
	}
}

func TestEncodeANSI_DoubleUnderlineAttr(t *testing.T) {
	grid := CellGrid{
		Rows: 1, Cols: 1,
		Cells: []Cell{{Char: 'X', Attrs: AttrDoubleUnderline}},
	}
	s := string(EncodeANSI(grid))
	if !strings.Contains(s, "\x1b[21m") {
		t.Errorf("missing double-underline SGR \\x1b[21m; got %q", s)
	}
}

func TestEncodeANSI_OverlineAttr(t *testing.T) {
	grid := CellGrid{
		Rows: 1, Cols: 1,
		Cells: []Cell{{Char: 'X', Attrs: AttrOverline}},
	}
	s := string(EncodeANSI(grid))
	if !strings.Contains(s, "\x1b[53m") {
		t.Errorf("missing overline SGR \\x1b[53m; got %q", s)
	}
}

func TestEncodeANSI_CurlyUnderlineAttr(t *testing.T) {
	grid := CellGrid{
		Rows: 1, Cols: 1,
		Cells: []Cell{{Char: 'X', Attrs: AttrCurlyUnderline}},
	}
	s := string(EncodeANSI(grid))
	if !strings.Contains(s, "\x1b[4:3m") {
		t.Errorf("missing curly-underline SGR \\x1b[4:3m; got %q", s)
	}
}
