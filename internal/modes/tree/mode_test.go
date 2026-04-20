package tree_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	"github.com/dhamidi/dmux/internal/pane"
	treemode "github.com/dhamidi/dmux/internal/modes/tree"
)

// ---- helpers ----------------------------------------------------------------

// stubTree returns a small three-level tree:
//
//	session-a
//	  win-0
//	    pane-0a
//	  win-1
//	session-b
//	  win-2
func stubTree() []treemode.TreeNode {
	return []treemode.TreeNode{
		{
			Kind: treemode.KindSession, ID: "s-a", Name: "session-a",
			Children: []treemode.TreeNode{
				{
					Kind: treemode.KindWindow, ID: "w-0", Name: "win-0",
					Children: []treemode.TreeNode{
						{Kind: treemode.KindPane, ID: "p-0a", Name: "pane-0a"},
					},
				},
				{Kind: treemode.KindWindow, ID: "w-1", Name: "win-1"},
			},
		},
		{
			Kind: treemode.KindSession, ID: "s-b", Name: "session-b",
			Children: []treemode.TreeNode{
				{Kind: treemode.KindWindow, ID: "w-2", Name: "win-2"},
			},
		},
	}
}

// testCanvas is a stub [modes.Canvas].
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

// rowText extracts the text of row r from the canvas.
func (c *testCanvas) rowText(r int) string {
	if r < 0 || r >= len(c.cells) {
		return ""
	}
	runes := make([]rune, len(c.cells[r]))
	for i, cell := range c.cells[r] {
		ch := cell.Char
		if ch == 0 {
			ch = ' '
		}
		runes[i] = ch
	}
	return string(runes)
}

// ---- PaneMode interface compliance -----------------------------------------

func TestImplementsPaneMode(t *testing.T) {
	var _ modes.PaneMode = treemode.New(stubTree(), nil, nil)
}

// ---- navigation ------------------------------------------------------------

func TestNavigation_InitialCursorAtZero(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	if got := m.Cursor(); got != 0 {
		t.Fatalf("initial cursor: want 0, got %d", got)
	}
}

func TestNavigation_MoveDown(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	m.Key(keys.Key{Code: keys.CodeDown})
	if got := m.Cursor(); got != 1 {
		t.Errorf("after Down: want 1, got %d", got)
	}
}

func TestNavigation_MoveUp(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	m.Key(keys.Key{Code: keys.CodeDown})
	m.Key(keys.Key{Code: keys.CodeUp})
	if got := m.Cursor(); got != 0 {
		t.Errorf("after Down+Up: want 0, got %d", got)
	}
}

func TestNavigation_ViKeys(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	m.Key(keys.Key{Code: keys.KeyCode('j')})
	if got := m.Cursor(); got != 1 {
		t.Errorf("'j': want 1, got %d", got)
	}
	m.Key(keys.Key{Code: keys.KeyCode('k')})
	if got := m.Cursor(); got != 0 {
		t.Errorf("'k': want 0, got %d", got)
	}
}

func TestNavigation_ClampAtTop(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	m.Key(keys.Key{Code: keys.CodeUp}) // already at top
	if got := m.Cursor(); got != 0 {
		t.Errorf("Up at top: want 0, got %d", got)
	}
}

func TestNavigation_ClampAtBottom(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	// 6 flat nodes; press Down many times
	for i := 0; i < 20; i++ {
		m.Key(keys.Key{Code: keys.CodeDown})
	}
	// stubTree flattened: s-a, w-0, p-0a, w-1, s-b, w-2 → 6 entries, last idx = 5
	if got := m.Cursor(); got != 5 {
		t.Errorf("Down past bottom: want 5, got %d", got)
	}
}

func TestNavigation_SelectedID_TracksCursor(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	if got := m.SelectedID(); got != "s-a" {
		t.Errorf("initial SelectedID: want s-a, got %q", got)
	}
	m.Key(keys.Key{Code: keys.CodeDown})
	if got := m.SelectedID(); got != "w-0" {
		t.Errorf("after Down SelectedID: want w-0, got %q", got)
	}
}

// ---- search ----------------------------------------------------------------

func TestSearch_SetSearchFilters(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	m.SetSearch("win")
	// Visible: win-0, win-1, win-2
	if got := m.SelectedID(); got != "w-0" {
		t.Errorf("after SetSearch('win') SelectedID: want w-0, got %q", got)
	}
	m.Key(keys.Key{Code: keys.CodeDown})
	if got := m.SelectedID(); got != "w-1" {
		t.Errorf("after Down in filtered list: want w-1, got %q", got)
	}
}

func TestSearch_CaseInsensitive(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	m.SetSearch("SESSION")
	if got := m.SelectedID(); got != "s-a" {
		t.Errorf("case-insensitive search: want s-a, got %q", got)
	}
}

func TestSearch_NoMatch_EmptyVisible(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	m.SetSearch("zzznomatch")
	if got := m.SelectedID(); got != "" {
		t.Errorf("no match: want empty ID, got %q", got)
	}
}

func TestSearch_SlashKeyEntersSearchMode(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	m.Key(keys.Key{Code: keys.KeyCode('/')})
	if !m.Searching() {
		t.Fatal("expected Searching() == true after '/'")
	}
	// Type "win"
	for _, ch := range "win" {
		m.Key(keys.Key{Code: keys.KeyCode(ch)})
	}
	if got := m.Search(); got != "win" {
		t.Errorf("search after typing: want %q, got %q", "win", got)
	}
}

func TestSearch_BackspaceRemovesChar(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	m.Key(keys.Key{Code: keys.KeyCode('/')})
	m.Key(keys.Key{Code: keys.KeyCode('w')})
	m.Key(keys.Key{Code: keys.KeyCode('i')})
	m.Key(keys.Key{Code: keys.CodeBackspace})
	if got := m.Search(); got != "w" {
		t.Errorf("after backspace: want %q, got %q", "w", got)
	}
}

func TestSearch_EscapeClearsQuery(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	m.Key(keys.Key{Code: keys.KeyCode('/')})
	m.Key(keys.Key{Code: keys.KeyCode('w')})
	m.Key(keys.Key{Code: keys.CodeEscape})
	if got := m.Search(); got != "" {
		t.Errorf("after Escape in search: want empty, got %q", got)
	}
	if m.Searching() {
		t.Error("Searching() should be false after Escape")
	}
}

func TestSearch_EnterConfirmsSearch(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	m.Key(keys.Key{Code: keys.KeyCode('/')})
	m.Key(keys.Key{Code: keys.KeyCode('w')})
	m.Key(keys.Key{Code: keys.CodeEnter})
	if m.Searching() {
		t.Error("Searching() should be false after Enter")
	}
	// Query is retained.
	if got := m.Search(); got != "w" {
		t.Errorf("query retained after Enter: want %q, got %q", "w", got)
	}
}

// ---- selection callback ----------------------------------------------------

func TestSelection_CallbackInvokedWithID(t *testing.T) {
	var selected string
	m := treemode.New(stubTree(), func(id string) { selected = id }, nil)
	m.Key(keys.Key{Code: keys.CodeDown}) // move to w-0
	out := m.Key(keys.Key{Code: keys.CodeEnter})

	if selected != "w-0" {
		t.Errorf("onSelect: want w-0, got %q", selected)
	}
	if out.Kind != modes.KindCloseMode {
		t.Errorf("Enter outcome: want KindCloseMode, got %v", out.Kind)
	}
}

func TestSelection_NilCallbackIsNoop(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	out := m.Key(keys.Key{Code: keys.CodeEnter})
	if out.Kind != modes.KindCloseMode {
		t.Errorf("nil onSelect: want KindCloseMode, got %v", out.Kind)
	}
}

func TestSelection_WithSearch_SelectsFilteredNode(t *testing.T) {
	var selected string
	m := treemode.New(stubTree(), func(id string) { selected = id }, nil)
	m.SetSearch("session-b")
	out := m.Key(keys.Key{Code: keys.CodeEnter})
	if selected != "s-b" {
		t.Errorf("select after search: want s-b, got %q", selected)
	}
	if out.Kind != modes.KindCloseMode {
		t.Errorf("Enter outcome: want KindCloseMode, got %v", out.Kind)
	}
}

// ---- Escape / q close mode -------------------------------------------------

func TestEscape_ClosesMode(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	out := m.Key(keys.Key{Code: keys.CodeEscape})
	if out.Kind != modes.KindCloseMode {
		t.Errorf("Escape: want KindCloseMode, got %v", out.Kind)
	}
}

func TestQ_ClosesMode(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	out := m.Key(keys.Key{Code: keys.KeyCode('q')})
	if out.Kind != modes.KindCloseMode {
		t.Errorf("q: want KindCloseMode, got %v", out.Kind)
	}
}

// ---- rendering -------------------------------------------------------------

func TestRender_ShowsNodeNames(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	canvas := newTestCanvas(6, 20)
	m.Render(canvas)

	// Row 0 should start with "session-a".
	row0 := canvas.rowText(0)
	if !startsWith(row0, "session-a") {
		t.Errorf("row 0: want prefix 'session-a', got %q", row0)
	}
	// Row 1 should be indented "win-0".
	row1 := canvas.rowText(1)
	if !startsWith(row1, "  win-0") {
		t.Errorf("row 1: want prefix '  win-0', got %q", row1)
	}
}

func TestRender_EmptyTree_NoPanic(t *testing.T) {
	m := treemode.New(nil, nil, nil)
	canvas := newTestCanvas(5, 20)
	m.Render(canvas) // must not panic
}

func TestRender_Preview_SplitsCanvas(t *testing.T) {
	called := false
	preview := func(id string) *pane.CellGrid {
		called = true
		grid := &pane.CellGrid{Rows: 4, Cols: 10}
		grid.Cells = make([]pane.Cell, 4*10)
		for i := range grid.Cells {
			grid.Cells[i] = pane.Cell{Char: 'X'}
		}
		return grid
	}
	m := treemode.New(stubTree(), nil, preview)
	canvas := newTestCanvas(4, 20)
	m.Render(canvas)

	if !called {
		t.Fatal("preview provider was not called")
	}
	// Right half (cols 10–19) should contain 'X'.
	if got := canvas.cells[0][10].Char; got != 'X' {
		t.Errorf("preview col 10: want 'X', got %q", got)
	}
}

func TestRender_NilPreview_UsesFullWidth(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	canvas := newTestCanvas(1, 20)
	m.Render(canvas)
	// All 20 columns should be written (not split).
	row := canvas.rowText(0)
	if len(row) != 20 {
		t.Errorf("full-width render: want 20 cols, got %d", len(row))
	}
}

// ---- Mouse -----------------------------------------------------------------

func TestMouse_Consumed(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	out := m.Mouse(keys.MouseEvent{})
	if out.Kind != modes.KindConsumed {
		t.Errorf("Mouse: want KindConsumed, got %v", out.Kind)
	}
}

// ---- Close -----------------------------------------------------------------

func TestClose_IsNoop(t *testing.T) {
	m := treemode.New(stubTree(), nil, nil)
	m.Close() // must not panic
}

// ---- helpers ----------------------------------------------------------------

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
