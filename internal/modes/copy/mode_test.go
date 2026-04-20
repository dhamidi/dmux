package copy_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	copymode "github.com/dhamidi/dmux/internal/modes/copy"
)

// stubScrollback is a test double for the Scrollback interface.
type stubScrollback struct {
	lines  []copymode.Line
	width  int
	height int
}

func (s *stubScrollback) Lines() []copymode.Line { return s.lines }
func (s *stubScrollback) Width() int             { return s.width }
func (s *stubScrollback) Height() int            { return s.height }

// makeLines builds []Line from plain strings, one cell per rune.
func makeLines(texts ...string) []copymode.Line {
	out := make([]copymode.Line, len(texts))
	for i, t := range texts {
		runes := []rune(t)
		row := make(copymode.Line, len(runes))
		for j, ch := range runes {
			row[j] = modes.Cell{Char: ch}
		}
		out[i] = row
	}
	return out
}

func newStub(texts ...string) *stubScrollback {
	return &stubScrollback{
		lines:  makeLines(texts...),
		width:  80,
		height: 10,
	}
}

// ---- PaneMode interface compliance -----------------------------------------

func TestImplementsPaneMode(t *testing.T) {
	sb := newStub("test")
	var _ modes.PaneMode = copymode.New(sb)
}

// ---- cursor movement -------------------------------------------------------

func TestCursorMovement_InitialPosition(t *testing.T) {
	sb := newStub("line0", "line1", "line2")
	m := copymode.New(sb)
	// cursor starts at the last (most recent) line
	if got := m.CursorRow(); got != 2 {
		t.Fatalf("initial curRow: want 2, got %d", got)
	}
}

func TestCursorMovement_Up(t *testing.T) {
	sb := newStub("line0", "line1", "line2")
	m := copymode.New(sb)

	m.Command("cursor-up")
	if got := m.CursorRow(); got != 1 {
		t.Errorf("after cursor-up: want 1, got %d", got)
	}
	m.Command("cursor-up")
	if got := m.CursorRow(); got != 0 {
		t.Errorf("after second cursor-up: want 0, got %d", got)
	}
	// Should not go above row 0.
	m.Command("cursor-up")
	if got := m.CursorRow(); got != 0 {
		t.Errorf("cursor-up at top: want 0, got %d", got)
	}
}

func TestCursorMovement_Down(t *testing.T) {
	sb := newStub("line0", "line1", "line2")
	m := copymode.New(sb)
	m.Command("history-top")

	m.Command("cursor-down")
	if got := m.CursorRow(); got != 1 {
		t.Errorf("after cursor-down: want 1, got %d", got)
	}
	m.Command("cursor-down")
	m.Command("cursor-down") // at last line; should clamp
	if got := m.CursorRow(); got != 2 {
		t.Errorf("cursor-down at bottom: want 2, got %d", got)
	}
}

func TestCursorMovement_LeftRight(t *testing.T) {
	sb := newStub("hello")
	m := copymode.New(sb)
	m.Command("start-of-line")

	if got := m.CursorCol(); got != 0 {
		t.Fatalf("start-of-line: want col=0, got %d", got)
	}
	m.Command("cursor-right")
	if got := m.CursorCol(); got != 1 {
		t.Errorf("after cursor-right: want col=1, got %d", got)
	}
	m.Command("end-of-line")
	// "hello" has 5 chars, indices 0–4.
	if got := m.CursorCol(); got != 4 {
		t.Errorf("end-of-line: want col=4, got %d", got)
	}
	m.Command("cursor-left")
	if got := m.CursorCol(); got != 3 {
		t.Errorf("after cursor-left: want col=3, got %d", got)
	}
	// Should not go beyond last column.
	m.Command("end-of-line")
	m.Command("cursor-right")
	if got := m.CursorCol(); got != 4 {
		t.Errorf("cursor-right at end: want col=4, got %d", got)
	}
}

func TestCursorMovement_PageUpDown(t *testing.T) {
	// 20 lines, height=10
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "line"
	}
	sb := &stubScrollback{lines: makeLines(lines...), width: 80, height: 10}
	m := copymode.New(sb) // starts at row 19

	m.Command("page-up")
	if got := m.CursorRow(); got != 9 {
		t.Errorf("after page-up: want 9, got %d", got)
	}
	m.Command("page-up")
	if got := m.CursorRow(); got != 0 {
		t.Errorf("second page-up clamped: want 0, got %d", got)
	}
	m.Command("page-down")
	if got := m.CursorRow(); got != 10 {
		t.Errorf("after page-down: want 10, got %d", got)
	}
	// page-down past end clamps.
	m.Command("page-down")
	m.Command("page-down")
	if got := m.CursorRow(); got != 19 {
		t.Errorf("page-down at end: want 19, got %d", got)
	}
}

func TestCursorMovement_HistoryTopBottom(t *testing.T) {
	sb := newStub("a", "b", "c")
	m := copymode.New(sb)

	m.Command("history-top")
	if got := m.CursorRow(); got != 0 {
		t.Errorf("history-top: want 0, got %d", got)
	}
	m.Command("history-bottom")
	if got := m.CursorRow(); got != 2 {
		t.Errorf("history-bottom: want 2, got %d", got)
	}
}

// ---- selection -------------------------------------------------------------

func TestSelection_SingleLine(t *testing.T) {
	sb := newStub("hello world")
	m := copymode.New(sb)
	m.Command("start-of-line")
	m.Command("begin-selection")

	row, col := m.SelectionAnchor()
	if row != 0 || col != 0 {
		t.Errorf("anchor: want (0,0), got (%d,%d)", row, col)
	}

	// Advance cursor to col 4 ("hello").
	for i := 0; i < 4; i++ {
		m.Command("cursor-right")
	}

	out := m.Command("copy-selection")
	if out.Kind != modes.KindCommand {
		t.Fatalf("copy-selection: want KindCommand, got %v", out.Kind)
	}
	cmd, ok := out.Cmd.(copymode.CopyCommand)
	if !ok {
		t.Fatalf("copy-selection: Cmd is not CopyCommand")
	}
	if cmd.Text != "hello" {
		t.Errorf("copy text: want %q, got %q", "hello", cmd.Text)
	}
	// Anchor cleared after copy.
	row, _ = m.SelectionAnchor()
	if row != -1 {
		t.Errorf("anchor after copy: want -1, got %d", row)
	}
}

func TestSelection_MultiLine(t *testing.T) {
	sb := newStub("foo", "bar", "baz")
	m := copymode.New(sb)
	m.Command("history-top")
	m.Command("begin-selection")
	m.Command("cursor-down")
	m.Command("end-of-line")

	out := m.Command("copy-selection")
	cmd := out.Cmd.(copymode.CopyCommand)
	if cmd.Text != "foo\nbar" {
		t.Errorf("multi-line selection: want %q, got %q", "foo\nbar", cmd.Text)
	}
}

func TestSelection_ClearSelection(t *testing.T) {
	sb := newStub("abc")
	m := copymode.New(sb)
	m.Command("begin-selection")
	m.Command("clear-selection")

	row, _ := m.SelectionAnchor()
	if row != -1 {
		t.Errorf("clear-selection: want anchor=-1, got %d", row)
	}
}

func TestSelection_NoSelectionReturnsEmpty(t *testing.T) {
	sb := newStub("hello")
	m := copymode.New(sb)
	// copy-selection without begin-selection should return empty text.
	out := m.Command("copy-selection")
	cmd := out.Cmd.(copymode.CopyCommand)
	if cmd.Text != "" {
		t.Errorf("copy without selection: want %q, got %q", "", cmd.Text)
	}
}

// ---- search ----------------------------------------------------------------

func TestSearch_Forward(t *testing.T) {
	sb := newStub("apple", "banana", "cherry", "banana")
	m := copymode.New(sb)
	m.Command("history-top")
	m.SetSearch("banana", true)

	if got := m.CursorRow(); got != 1 {
		t.Errorf("SetSearch forward: want row=1, got %d", got)
	}
	m.Command("search-again")
	if got := m.CursorRow(); got != 3 {
		t.Errorf("search-again forward: want row=3, got %d", got)
	}
}

func TestSearch_Backward(t *testing.T) {
	sb := newStub("apple", "banana", "cherry", "banana")
	m := copymode.New(sb) // starts at row 3 ("banana")
	m.SetSearch("banana", false)
	// From row 3, backwards: row 1 has "banana".
	if got := m.CursorRow(); got != 1 {
		t.Errorf("SetSearch backward: want row=1, got %d", got)
	}
}

func TestSearch_WrapAround(t *testing.T) {
	sb := newStub("apple", "banana", "cherry")
	m := copymode.New(sb)
	m.Command("history-bottom") // row 2
	m.SetSearch("apple", true)
	// From row 2 forward, wraps to row 0.
	if got := m.CursorRow(); got != 0 {
		t.Errorf("SetSearch wrap: want row=0, got %d", got)
	}
}

func TestSearch_NoMatch(t *testing.T) {
	sb := newStub("apple", "banana")
	m := copymode.New(sb)
	m.Command("history-top")
	m.SetSearch("xyz", true)
	// Cursor should not move.
	if got := m.CursorRow(); got != 0 {
		t.Errorf("no match: cursor should stay at 0, got %d", got)
	}
}

func TestSearch_Reverse(t *testing.T) {
	sb := newStub("apple", "banana", "cherry", "apple")
	m := copymode.New(sb)
	m.Command("history-top")
	m.SetSearch("apple", true)  // finds row 3 (wraps)
	m.Command("search-reverse") // goes backward: from row 3 → row 0
	if got := m.CursorRow(); got != 0 {
		t.Errorf("search-reverse: want row=0, got %d", got)
	}
}

// ---- Key event mapping -----------------------------------------------------

func TestKey_Escape_ClosesMode(t *testing.T) {
	sb := newStub("test")
	m := copymode.New(sb)
	out := m.Key(keys.Key{Code: keys.CodeEscape})
	if out.Kind != modes.KindCloseMode {
		t.Errorf("Escape: want KindCloseMode, got %v", out.Kind)
	}
}

func TestKey_Q_ClosesMode(t *testing.T) {
	sb := newStub("test")
	m := copymode.New(sb)
	out := m.Key(keys.Key{Code: keys.KeyCode('q')})
	if out.Kind != modes.KindCloseMode {
		t.Errorf("q: want KindCloseMode, got %v", out.Kind)
	}
}

func TestKey_ArrowKeys_MoveCursor(t *testing.T) {
	sb := newStub("abc", "def", "ghi")
	m := copymode.New(sb) // row 2
	m.Key(keys.Key{Code: keys.CodeUp})
	if got := m.CursorRow(); got != 1 {
		t.Errorf("Up arrow: want row=1, got %d", got)
	}
	m.Key(keys.Key{Code: keys.CodeDown})
	if got := m.CursorRow(); got != 2 {
		t.Errorf("Down arrow: want row=2, got %d", got)
	}
}

func TestKey_ViKeys_MoveCursor(t *testing.T) {
	sb := newStub("abc", "def", "ghi")
	m := copymode.New(sb) // row 2
	m.Key(keys.Key{Code: keys.KeyCode('k')})
	if got := m.CursorRow(); got != 1 {
		t.Errorf("'k': want row=1, got %d", got)
	}
	m.Key(keys.Key{Code: keys.KeyCode('j')})
	if got := m.CursorRow(); got != 2 {
		t.Errorf("'j': want row=2, got %d", got)
	}
}

func TestKey_Unknown_Consumed(t *testing.T) {
	sb := newStub("test")
	m := copymode.New(sb)
	out := m.Key(keys.Key{Code: keys.KeyCode('z')})
	if out.Kind != modes.KindConsumed {
		t.Errorf("unknown key: want KindConsumed, got %v", out.Kind)
	}
}

func TestKey_Cancel_ClosesMode(t *testing.T) {
	sb := newStub("test")
	m := copymode.New(sb)
	out := m.Command("cancel")
	if out.Kind != modes.KindCloseMode {
		t.Errorf("cancel: want KindCloseMode, got %v", out.Kind)
	}
}

// ---- rendering -------------------------------------------------------------

// testCanvas is a stub Canvas for render tests.
type testCanvas struct {
	size  modes.Size
	cells [][]modes.Cell
}

func newTestCanvas(rows, cols int) *testCanvas {
	c := &testCanvas{size: modes.Size{Rows: rows, Cols: cols}}
	c.cells = make([][]modes.Cell, rows)
	for i := range c.cells {
		c.cells[i] = make([]modes.Cell, cols)
	}
	return c
}

func (c *testCanvas) Size() modes.Size { return c.size }
func (c *testCanvas) Set(col, row int, cell modes.Cell) {
	if row >= 0 && row < c.size.Rows && col >= 0 && col < c.size.Cols {
		c.cells[row][col] = cell
	}
}

func TestRender_Basic(t *testing.T) {
	sb := newStub("abc", "def")
	m := copymode.New(sb)
	m.Command("history-top")

	canvas := newTestCanvas(2, 3)
	m.Render(canvas)

	want := []string{"abc", "def"}
	for row, wantStr := range want {
		for col, wantCh := range wantStr {
			got := canvas.cells[row][col].Char
			if got != wantCh {
				t.Errorf("cell[%d][%d]: want %q, got %q", row, col, wantCh, got)
			}
		}
	}
}

func TestRender_ShorterLines_PaddedWithSpace(t *testing.T) {
	sb := newStub("hi")
	m := copymode.New(sb)
	m.Command("history-top")

	canvas := newTestCanvas(1, 5)
	m.Render(canvas)

	// Columns beyond "hi" should be spaces.
	for col := 2; col < 5; col++ {
		if got := canvas.cells[0][col].Char; got != ' ' {
			t.Errorf("col %d: want ' ', got %q", col, got)
		}
	}
}

func TestRender_ViewportScrolls(t *testing.T) {
	// 5 lines, visible height=2; cursor at row 4.
	sb := &stubScrollback{lines: makeLines("r0", "r1", "r2", "r3", "r4"), width: 2, height: 2}
	m := copymode.New(sb) // cursor at row 4

	canvas := newTestCanvas(2, 2)
	m.Render(canvas)

	// Viewport should show rows 3 and 4.
	want := []string{"r3", "r4"}
	for row, wantStr := range want {
		for col, wantCh := range wantStr {
			got := canvas.cells[row][col].Char
			if got != wantCh {
				t.Errorf("viewport cell[%d][%d]: want %q, got %q", row, col, wantCh, got)
			}
		}
	}
}
