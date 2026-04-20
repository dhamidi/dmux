package term

import (
	"bytes"
	"strings"
	"testing"
)

// stubSize returns a SizeFunc that always returns the given dimensions.
func stubSize(rows, cols int) SizeFunc {
	return func() (int, int, error) { return rows, cols, nil }
}

// newTestTerm creates a Term backed by an in-memory buffer.
// RawMode is nil so no OS terminal calls are made.
func newTestTerm(t *testing.T, rows, cols int) (*Term, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	term, err := Open(Config{
		Out:  &buf,
		Size: stubSize(rows, cols),
		// RawMode intentionally nil: no OS interaction in tests.
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return term, &buf
}

func TestOpenRequiresOut(t *testing.T) {
	_, err := Open(Config{Size: stubSize(24, 80)})
	if err == nil {
		t.Fatal("expected error when Out is nil")
	}
}

func TestOpenRequiresSize(t *testing.T) {
	var buf bytes.Buffer
	_, err := Open(Config{Out: &buf})
	if err == nil {
		t.Fatal("expected error when Size is nil")
	}
}

func TestSize(t *testing.T) {
	term, _ := newTestTerm(t, 24, 80)
	rows, cols, err := term.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if rows != 24 || cols != 80 {
		t.Errorf("Size() = (%d, %d), want (24, 80)", rows, cols)
	}
}

func TestSizeResize(t *testing.T) {
	calls := 0
	sizes := [][2]int{{24, 80}, {30, 100}}
	sizeFn := func() (int, int, error) {
		s := sizes[calls]
		calls++
		return s[0], s[1], nil
	}
	var buf bytes.Buffer
	term, err := Open(Config{Out: &buf, Size: sizeFn})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Second Size() call returns new dimensions.
	rows, cols, err := term.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if rows != 30 || cols != 100 {
		t.Errorf("Size() = (%d, %d), want (30, 100)", rows, cols)
	}
}

func TestFlushWritesEscapeSequences(t *testing.T) {
	term, buf := newTestTerm(t, 1, 1)

	term.SetCell(0, 0, Cell{Rune: 'X'})
	if err := term.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	out := buf.String()
	// Must hide cursor at start.
	if !strings.Contains(out, "\x1b[?25l") {
		t.Error("expected hide-cursor sequence \\x1b[?25l")
	}
	// Must position cursor at row 1, col 1 (1-based).
	if !strings.Contains(out, "\x1b[1;1H") {
		t.Error("expected cursor-position sequence \\x1b[1;1H")
	}
	// Must contain the character.
	if !strings.Contains(out, "X") {
		t.Error("expected character 'X' in output")
	}
	// Must show cursor at end.
	if !strings.Contains(out, "\x1b[?25h") {
		t.Error("expected show-cursor sequence \\x1b[?25h")
	}
}

func TestFlushDiffSkipsUnchangedCells(t *testing.T) {
	term, buf := newTestTerm(t, 2, 2)

	// First flush: set only cell (0,0).
	term.SetCell(0, 0, Cell{Rune: 'A'})
	if err := term.Flush(); err != nil {
		t.Fatalf("first Flush: %v", err)
	}
	buf.Reset()

	// Second flush: change only cell (1,1).
	term.SetCell(1, 1, Cell{Rune: 'B'})
	if err := term.Flush(); err != nil {
		t.Fatalf("second Flush: %v", err)
	}

	out := buf.String()
	// Must position at (2,2) for the changed cell.
	if !strings.Contains(out, "\x1b[2;2H") {
		t.Error("expected cursor-position \\x1b[2;2H for cell (1,1)")
	}
	// Must NOT reposition at (1,1) — cell (0,0) did not change.
	if strings.Contains(out, "\x1b[1;1H") {
		t.Error("unexpected cursor-position \\x1b[1;1H: cell (0,0) did not change")
	}
	if !strings.Contains(out, "B") {
		t.Error("expected character 'B' in second flush output")
	}
}

func TestFlushAdjacentCellsSkipCursorMove(t *testing.T) {
	term, buf := newTestTerm(t, 1, 3)

	term.SetCell(0, 0, Cell{Rune: 'A'})
	term.SetCell(0, 1, Cell{Rune: 'B'})
	term.SetCell(0, 2, Cell{Rune: 'C'})
	if err := term.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	out := buf.String()
	// Only one cursor-move for the first cell.
	count := strings.Count(out, "\x1b[")
	// Expected sequences: hide-cursor, one CUP, one SGR×3, reset, show-cursor
	// At most one CUP move sequence for position (1,1).
	moveCount := strings.Count(out, "\x1b[1;1H")
	if moveCount != 1 {
		t.Errorf("expected exactly one cursor-move to (1,1), got %d; full output: %q", moveCount, out)
	}
	// No move to (1,2) or (1,3) because they are adjacent.
	if strings.Contains(out, "\x1b[1;2H") || strings.Contains(out, "\x1b[1;3H") {
		t.Errorf("unexpected extra cursor-move in adjacent cells; count=%d out=%q", count, out)
	}
}

func TestFlushPaletteColor(t *testing.T) {
	term, buf := newTestTerm(t, 1, 1)

	term.SetCell(0, 0, Cell{
		Rune: 'Z',
		FG:   PaletteColor(196), // bright red
		BG:   PaletteColor(0),   // black
	})
	if err := term.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "38;5;196") {
		t.Errorf("expected 38;5;196 (palette FG) in output: %q", out)
	}
	if !strings.Contains(out, "48;5;0") {
		t.Errorf("expected 48;5;0 (palette BG) in output: %q", out)
	}
}

func TestFlushRGBColor(t *testing.T) {
	term, buf := newTestTerm(t, 1, 1)

	term.SetCell(0, 0, Cell{
		Rune: 'R',
		FG:   RGBColor(255, 128, 0),
		BG:   RGBColor(0, 0, 128),
	})
	if err := term.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "38;2;255;128;0") {
		t.Errorf("expected 38;2;255;128;0 (RGB FG) in output: %q", out)
	}
	if !strings.Contains(out, "48;2;0;0;128") {
		t.Errorf("expected 48;2;0;0;128 (RGB BG) in output: %q", out)
	}
}

func TestFlushAttributes(t *testing.T) {
	term, buf := newTestTerm(t, 1, 1)

	term.SetCell(0, 0, Cell{
		Rune: 'B',
		Attr: AttrBold | AttrUnderline,
	})
	if err := term.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, ";1;") && !strings.Contains(out, "[1;") {
		t.Errorf("expected bold (SGR 1) in output: %q", out)
	}
	if !strings.Contains(out, ";4;") && !strings.Contains(out, ";4m") {
		t.Errorf("expected underline (SGR 4) in output: %q", out)
	}
}

func TestSetCellOutOfBoundsIgnored(t *testing.T) {
	term, buf := newTestTerm(t, 2, 2)

	// These should not panic.
	term.SetCell(-1, 0, Cell{Rune: 'X'})
	term.SetCell(0, -1, Cell{Rune: 'X'})
	term.SetCell(2, 0, Cell{Rune: 'X'})
	term.SetCell(0, 2, Cell{Rune: 'X'})

	if err := term.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	// None of the out-of-bounds runes should appear.
	if strings.Count(buf.String(), "X") > 0 {
		t.Errorf("out-of-bounds cell 'X' appeared in output: %q", buf.String())
	}
}

func TestClose(t *testing.T) {
	restoreCalled := false
	var buf bytes.Buffer
	term, err := Open(Config{
		Out:  &buf,
		Size: stubSize(24, 80),
		RawMode: func() (func() error, error) {
			return func() error {
				restoreCalled = true
				return nil
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := term.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !restoreCalled {
		t.Error("expected restore function to be called on Close")
	}
	// Second Close should be a no-op (not call restore again).
	restoreCalled = false
	if err := term.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if restoreCalled {
		t.Error("second Close must not call restore again")
	}
}

func TestClear(t *testing.T) {
	term, buf := newTestTerm(t, 1, 2)

	term.SetCell(0, 0, Cell{Rune: 'A'})
	term.SetCell(0, 1, Cell{Rune: 'B'})
	term.Clear()
	if err := term.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	// After Clear, cells are spaces; 'A' and 'B' should not appear.
	out := buf.String()
	if strings.Contains(out, "A") || strings.Contains(out, "B") {
		t.Errorf("expected cleared cells, but got: %q", out)
	}
}

func TestRawModeNilSkipped(t *testing.T) {
	// Open with nil RawMode must not error.
	var buf bytes.Buffer
	term, err := Open(Config{Out: &buf, Size: stubSize(10, 10)})
	if err != nil {
		t.Fatalf("Open with nil RawMode: %v", err)
	}
	// Close with no raw mode should be a no-op.
	if err := term.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
