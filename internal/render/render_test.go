package render_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/render"
)

// fakePane is a test double for render.Pane.
type fakePane struct {
	bounds render.Rect
	grid   render.CellGrid
}

func (f *fakePane) Bounds() render.Rect    { return f.bounds }
func (f *fakePane) Snapshot() render.CellGrid { return f.grid }

// fakeStatusLine is a test double for render.StatusLine.
type fakeStatusLine struct {
	cells []render.Cell
}

func (f *fakeStatusLine) Render(width int) []render.Cell {
	out := make([]render.Cell, width)
	copy(out, f.cells)
	return out
}

// fakeOverlay is a test double for render.Overlay.
type fakeOverlay struct {
	rect  render.Rect
	cells []render.Cell
}

func (f *fakeOverlay) Rect() render.Rect { return f.rect }
func (f *fakeOverlay) Render(dst []render.Cell) {
	copy(dst, f.cells)
}

// makeGrid builds a CellGrid filled with a single character.
func makeGrid(rows, cols int, ch rune) render.CellGrid {
	cells := make([]render.Cell, rows*cols)
	for i := range cells {
		cells[i] = render.Cell{Char: ch}
	}
	return render.CellGrid{Rows: rows, Cols: cols, Cells: cells}
}

// cellAt is a helper to read a cell from a composed grid.
func cellAt(g render.CellGrid, row, col int) render.Cell {
	return g.Cells[row*g.Cols+col]
}

// TestCompose_EmptyProducesSpaces verifies that composing with no panes
// yields a grid filled with space characters.
func TestCompose_EmptyProducesSpaces(t *testing.T) {
	r := render.New(render.Config{Rows: 3, Cols: 5})
	grid := r.Compose(nil, nil)

	if grid.Rows != 3 || grid.Cols != 5 {
		t.Fatalf("got %dx%d, want 3x5", grid.Rows, grid.Cols)
	}
	for i, c := range grid.Cells {
		if c.Char != ' ' {
			t.Errorf("cell[%d] = %q, want ' '", i, c.Char)
		}
	}
}

// TestCompose_SinglePane verifies that a pane's cells are blitted into
// the correct region of the output grid.
func TestCompose_SinglePane(t *testing.T) {
	r := render.New(render.Config{Rows: 4, Cols: 8})

	pane := &fakePane{
		bounds: render.Rect{X: 2, Y: 1, Width: 3, Height: 2},
		grid:   makeGrid(2, 3, 'X'),
	}
	placement := render.PanePlacement{
		Pane: pane,
		Rect: pane.bounds,
	}

	grid := r.Compose([]render.PanePlacement{placement}, nil)

	// Cells inside the pane rect should be 'X'.
	for row := 1; row <= 2; row++ {
		for col := 2; col <= 4; col++ {
			if got := cellAt(grid, row, col).Char; got != 'X' {
				t.Errorf("cell(%d,%d) = %q, want 'X'", row, col, got)
			}
		}
	}
	// A cell outside the pane rect should be ' '.
	if got := cellAt(grid, 0, 0).Char; got != ' ' {
		t.Errorf("cell(0,0) = %q, want ' '", got)
	}
}

// TestCompose_TwoPanesOverlap verifies that later panes overwrite earlier
// ones in overlapping regions.
func TestCompose_TwoPanesOverlap(t *testing.T) {
	r := render.New(render.Config{Rows: 4, Cols: 8})

	first := &fakePane{
		bounds: render.Rect{X: 0, Y: 0, Width: 4, Height: 4},
		grid:   makeGrid(4, 4, 'A'),
	}
	second := &fakePane{
		bounds: render.Rect{X: 2, Y: 0, Width: 4, Height: 4},
		grid:   makeGrid(4, 4, 'B'),
	}

	grid := r.Compose([]render.PanePlacement{
		{Pane: first, Rect: first.bounds},
		{Pane: second, Rect: second.bounds},
	}, nil)

	// Column 0-1: only first pane → 'A'
	if got := cellAt(grid, 0, 0).Char; got != 'A' {
		t.Errorf("cell(0,0) = %q, want 'A'", got)
	}
	// Columns 2-5: second pane overwrites → 'B'
	if got := cellAt(grid, 0, 2).Char; got != 'B' {
		t.Errorf("cell(0,2) = %q, want 'B'", got)
	}
	if got := cellAt(grid, 0, 5).Char; got != 'B' {
		t.Errorf("cell(0,5) = %q, want 'B'", got)
	}
}

// TestCompose_StatusLine verifies that the status line occupies the last row
// and pane content does not bleed into it.
func TestCompose_StatusLine(t *testing.T) {
	status := &fakeStatusLine{
		cells: []render.Cell{{Char: 'S'}, {Char: 'T'}},
	}
	r := render.New(render.Config{
		Rows:       4,
		Cols:       5,
		Status:     status,
		StatusRows: 1,
	})

	// Pane fills the full 4 rows, but only the first 3 should be used.
	pane := &fakePane{
		bounds: render.Rect{X: 0, Y: 0, Width: 5, Height: 4},
		grid:   makeGrid(4, 5, 'P'),
	}

	grid := r.Compose([]render.PanePlacement{{Pane: pane, Rect: pane.bounds}}, nil)

	// Rows 0-2 should have pane cells.
	for row := 0; row < 3; row++ {
		if got := cellAt(grid, row, 0).Char; got != 'P' {
			t.Errorf("row %d col 0 = %q, want 'P'", row, got)
		}
	}
	// Row 3 (status) columns 0-1 should have 'S' and 'T'.
	if got := cellAt(grid, 3, 0).Char; got != 'S' {
		t.Errorf("status row col 0 = %q, want 'S'", got)
	}
	if got := cellAt(grid, 3, 1).Char; got != 'T' {
		t.Errorf("status row col 1 = %q, want 'T'", got)
	}
}

// TestCompose_OverlayOnTopOfPane verifies that overlays are applied after
// pane content and overwrite cells in their rect.
func TestCompose_OverlayOnTopOfPane(t *testing.T) {
	r := render.New(render.Config{Rows: 4, Cols: 8})

	pane := &fakePane{
		bounds: render.Rect{X: 0, Y: 0, Width: 8, Height: 4},
		grid:   makeGrid(4, 8, 'P'),
	}
	overlay := &fakeOverlay{
		rect:  render.Rect{X: 2, Y: 1, Width: 2, Height: 2},
		cells: []render.Cell{{Char: 'O'}, {Char: 'O'}, {Char: 'O'}, {Char: 'O'}},
	}

	grid := r.Compose(
		[]render.PanePlacement{{Pane: pane, Rect: pane.bounds}},
		[]render.Overlay{overlay},
	)

	// Inside overlay rect: 'O'
	if got := cellAt(grid, 1, 2).Char; got != 'O' {
		t.Errorf("overlay cell(1,2) = %q, want 'O'", got)
	}
	if got := cellAt(grid, 2, 3).Char; got != 'O' {
		t.Errorf("overlay cell(2,3) = %q, want 'O'", got)
	}
	// Outside overlay rect: pane cell 'P'
	if got := cellAt(grid, 0, 0).Char; got != 'P' {
		t.Errorf("pane cell(0,0) = %q, want 'P'", got)
	}
}

// TestCompose_PaneZeroCharBecomesSpace verifies that zero-rune cells in
// pane snapshots are normalised to spaces in the output.
func TestCompose_PaneZeroCharBecomesSpace(t *testing.T) {
	r := render.New(render.Config{Rows: 2, Cols: 2})

	cells := []render.Cell{{Char: 0}, {Char: 'A'}, {Char: 'B'}, {Char: 0}}
	pane := &fakePane{
		bounds: render.Rect{X: 0, Y: 0, Width: 2, Height: 2},
		grid:   render.CellGrid{Rows: 2, Cols: 2, Cells: cells},
	}

	grid := r.Compose([]render.PanePlacement{{Pane: pane, Rect: pane.bounds}}, nil)

	if got := cellAt(grid, 0, 0).Char; got != ' ' {
		t.Errorf("cell(0,0) = %q, want ' ' (normalised from zero)", got)
	}
	if got := cellAt(grid, 0, 1).Char; got != 'A' {
		t.Errorf("cell(0,1) = %q, want 'A'", got)
	}
	if got := cellAt(grid, 1, 1).Char; got != ' ' {
		t.Errorf("cell(1,1) = %q, want ' ' (normalised from zero)", got)
	}
}

// TestCompose_PaneClippedAtGridBoundary verifies that panes positioned
// partially outside the grid do not cause out-of-bounds writes.
func TestCompose_PaneClippedAtGridBoundary(t *testing.T) {
	r := render.New(render.Config{Rows: 3, Cols: 3})

	// Pane starts at (2,2) with a 2x2 grid — overlaps the corner only.
	pane := &fakePane{
		bounds: render.Rect{X: 2, Y: 2, Width: 2, Height: 2},
		grid:   makeGrid(2, 2, 'C'),
	}

	// Should not panic.
	grid := r.Compose([]render.PanePlacement{{Pane: pane, Rect: pane.bounds}}, nil)

	// Only (2,2) should have 'C'; others are space.
	if got := cellAt(grid, 2, 2).Char; got != 'C' {
		t.Errorf("cell(2,2) = %q, want 'C'", got)
	}
	if got := cellAt(grid, 0, 0).Char; got != ' ' {
		t.Errorf("cell(0,0) = %q, want ' '", got)
	}
}

// TestCompose_BorderLineSet verifies that two panes side-by-side produce
// border characters matching the configured pane-border-lines set.
func TestCompose_BorderLineSet(t *testing.T) {
	tests := []struct {
		borderLines string
		wantVert    rune
		wantHoriz   rune
	}{
		{"single", '│', '─'},
		{"double", '║', '═'},
		{"heavy", '┃', '━'},
		{"simple", '|', '-'},
		{"padded", ' ', ' '},
	}

	for _, tc := range tests {
		t.Run(tc.borderLines, func(t *testing.T) {
			r := render.New(render.Config{
				Rows: 4,
				Cols: 9,
				Theme: render.Theme{BorderLines: tc.borderLines},
			})

			// Left pane occupies cols 0-4 (col 4 is the vertical border).
			left := &fakePane{
				bounds: render.Rect{X: 0, Y: 0, Width: 5, Height: 4},
				grid:   makeGrid(4, 5, 'L'),
			}
			// Right pane occupies cols 5-8.
			right := &fakePane{
				bounds: render.Rect{X: 5, Y: 0, Width: 4, Height: 4},
				grid:   makeGrid(4, 4, 'R'),
			}

			grid := r.Compose([]render.PanePlacement{
				{Pane: left, Rect: left.bounds},
				{Pane: right, Rect: right.bounds},
			}, nil)

			// The vertical border at col 4 (right edge of left pane) should
			// contain the Vertical character from the selected set for rows
			// that are not also a horizontal border (i.e. not the bottom row).
			for row := 0; row < 3; row++ {
				if got := cellAt(grid, row, 4).Char; got != tc.wantVert {
					t.Errorf("border cell(%d,4) = %q, want %q", row, got, tc.wantVert)
				}
			}
		})
	}
}

// TestCompose_BorderLineSet_TopBottom verifies that a top/bottom two-pane layout
// produces horizontal border characters.
func TestCompose_BorderLineSet_TopBottom(t *testing.T) {
	r := render.New(render.Config{
		Rows: 5,
		Cols: 6,
		Theme: render.Theme{BorderLines: "double"},
	})

	// Top pane occupies rows 0-2 (row 2 is the horizontal border).
	top := &fakePane{
		bounds: render.Rect{X: 0, Y: 0, Width: 6, Height: 3},
		grid:   makeGrid(3, 6, 'T'),
	}
	// Bottom pane occupies rows 3-4.
	bottom := &fakePane{
		bounds: render.Rect{X: 0, Y: 3, Width: 6, Height: 2},
		grid:   makeGrid(2, 6, 'B'),
	}

	grid := r.Compose([]render.PanePlacement{
		{Pane: top, Rect: top.bounds},
		{Pane: bottom, Rect: bottom.bounds},
	}, nil)

	// Row 2 (bottom of top pane) should be horizontal double-line border '═',
	// except the last column which is the right edge of the top pane.
	for col := 0; col < 5; col++ {
		if got := cellAt(grid, 2, col).Char; got != '═' {
			t.Errorf("border cell(2,%d) = %q, want '═'", col, got)
		}
	}
}

// TestCompose_BorderLineSet_NoBorderWhenEmpty verifies that no border is drawn
// when BorderLines is empty (default).
func TestCompose_BorderLineSet_NoBorderWhenEmpty(t *testing.T) {
	r := render.New(render.Config{Rows: 4, Cols: 9})

	left := &fakePane{
		bounds: render.Rect{X: 0, Y: 0, Width: 5, Height: 4},
		grid:   makeGrid(4, 5, 'L'),
	}
	right := &fakePane{
		bounds: render.Rect{X: 5, Y: 0, Width: 4, Height: 4},
		grid:   makeGrid(4, 4, 'R'),
	}

	grid := r.Compose([]render.PanePlacement{
		{Pane: left, Rect: left.bounds},
		{Pane: right, Rect: right.bounds},
	}, nil)

	// Without BorderLines, col 4 should still have the pane content 'L'.
	for row := 0; row < 4; row++ {
		if got := cellAt(grid, row, 4).Char; got != 'L' {
			t.Errorf("cell(%d,4) = %q, want 'L' (no border)", row, got)
		}
	}
}

// TestCompose_PaneBorderStatus_Top verifies that pane-border-status "top"
// places the default format label (#{pane_index}) on the horizontal border
// immediately above each pane, centered within the pane width.
func TestCompose_PaneBorderStatus_Top(t *testing.T) {
	r := render.New(render.Config{
		Rows: 6,
		Cols: 8,
		Theme: render.Theme{
			BorderLines:      "single",
			PaneBorderStatus: "top",
		},
	})

	// Top pane: rows 0-2 (row 2 is the horizontal border).
	top := &fakePane{
		bounds: render.Rect{X: 0, Y: 0, Width: 8, Height: 3},
		grid:   makeGrid(3, 8, 'T'),
	}
	// Bottom pane: rows 3-5 (row 5 is the horizontal border).
	bottom := &fakePane{
		bounds: render.Rect{X: 0, Y: 3, Width: 8, Height: 3},
		grid:   makeGrid(3, 8, 'B'),
	}

	grid := r.Compose([]render.PanePlacement{
		{Pane: top, Rect: top.bounds, PaneIndex: 0},
		{Pane: bottom, Rect: bottom.bounds, PaneIndex: 1},
	}, nil)

	// "top" of the bottom pane (PaneIndex 1) is the border above it: row 2.
	// maxWidth = 8-2 = 6, label "1" len=1, leftPad = (6-1)/2 = 2
	// label char at col startCol+leftPad = 1+2 = 3
	if got := cellAt(grid, 2, 3).Char; got != '1' {
		t.Errorf("border label cell(2,3) = %q, want '1'", got)
	}
	// Columns to the left of the label should be horizontal border characters.
	for col := 1; col < 3; col++ {
		if got := cellAt(grid, 2, col).Char; got != '─' {
			t.Errorf("border pad cell(2,%d) = %q, want '─'", col, got)
		}
	}

	// "top" of the top pane (PaneIndex 0) is row -1: no label should be written.
	// Row 0 should still contain pane content 'T', not a label character.
	if got := cellAt(grid, 0, 3).Char; got != 'T' {
		t.Errorf("top-pane row 0 cell(0,3) = %q, want 'T' (no top border for first pane)", got)
	}
}

// TestCompose_PaneBorderStatus_Bottom verifies that pane-border-status "bottom"
// places the label on the pane's own bottom horizontal border row.
func TestCompose_PaneBorderStatus_Bottom(t *testing.T) {
	r := render.New(render.Config{
		Rows: 6,
		Cols: 8,
		Theme: render.Theme{
			BorderLines:      "single",
			PaneBorderStatus: "bottom",
		},
	})

	top := &fakePane{
		bounds: render.Rect{X: 0, Y: 0, Width: 8, Height: 3},
		grid:   makeGrid(3, 8, 'T'),
	}
	bottom := &fakePane{
		bounds: render.Rect{X: 0, Y: 3, Width: 8, Height: 3},
		grid:   makeGrid(3, 8, 'B'),
	}

	grid := r.Compose([]render.PanePlacement{
		{Pane: top, Rect: top.bounds, PaneIndex: 0},
		{Pane: bottom, Rect: bottom.bounds, PaneIndex: 1},
	}, nil)

	// "bottom" of top pane (PaneIndex 0) is row 2.
	// Label "0": maxWidth=6, leftPad=(6-1)/2=2, col=1+2=3
	if got := cellAt(grid, 2, 3).Char; got != '0' {
		t.Errorf("border label cell(2,3) = %q, want '0'", got)
	}

	// "bottom" of bottom pane (PaneIndex 1) is row 5.
	// Label "1" at col 3.
	if got := cellAt(grid, 5, 3).Char; got != '1' {
		t.Errorf("border label cell(5,3) = %q, want '1'", got)
	}
}

// TestCompose_PaneBorderStatus_Off verifies that pane-border-status "off"
// (and empty string) leaves borders unchanged with no label overlay.
func TestCompose_PaneBorderStatus_Off(t *testing.T) {
	for _, status := range []string{"off", ""} {
		t.Run(status, func(t *testing.T) {
			r := render.New(render.Config{
				Rows: 6,
				Cols: 8,
				Theme: render.Theme{
					BorderLines:      "single",
					PaneBorderStatus: status,
				},
			})

			top := &fakePane{
				bounds: render.Rect{X: 0, Y: 0, Width: 8, Height: 3},
				grid:   makeGrid(3, 8, 'T'),
			}
			bottom := &fakePane{
				bounds: render.Rect{X: 0, Y: 3, Width: 8, Height: 3},
				grid:   makeGrid(3, 8, 'B'),
			}

			grid := r.Compose([]render.PanePlacement{
				{Pane: top, Rect: top.bounds, PaneIndex: 0},
				{Pane: bottom, Rect: bottom.bounds, PaneIndex: 1},
			}, nil)

			// Row 2 should be a plain horizontal border '─' with no label digits.
			for col := 0; col < 7; col++ {
				if got := cellAt(grid, 2, col).Char; got != '─' {
					t.Errorf("cell(2,%d) = %q, want '─' (no label when status=%q)", col, got, status)
				}
			}
		})
	}
}

// TestCompose_PaneBorderStatus_Truncate verifies that labels longer than
// pane width minus 2 are truncated to fit.
func TestCompose_PaneBorderStatus_Truncate(t *testing.T) {
	r := render.New(render.Config{
		Rows: 6,
		Cols: 6,
		Theme: render.Theme{
			BorderLines:      "single",
			PaneBorderStatus: "bottom",
			PaneBorderFormat: "hello",
		},
	})

	// Pane width 6 → maxWidth = 4. "hello" (5 chars) truncated to "hell".
	top := &fakePane{
		bounds: render.Rect{X: 0, Y: 0, Width: 6, Height: 3},
		grid:   makeGrid(3, 6, 'T'),
	}

	grid := r.Compose([]render.PanePlacement{
		{Pane: top, Rect: top.bounds, PaneIndex: 0},
	}, nil)

	// The border row is row 2. Interior cols 1-4 (maxWidth=4).
	// "hell" fills all 4 interior cols (leftPad=0).
	want := []rune{'h', 'e', 'l', 'l'}
	for i, wantCh := range want {
		col := 1 + i
		if got := cellAt(grid, 2, col).Char; got != wantCh {
			t.Errorf("truncated label cell(2,%d) = %q, want %q", col, got, wantCh)
		}
	}
}

// TestBlendStyle_DefaultColorsReplaced verifies that ColorDefault fg and bg
// are replaced by the supplied defaults.
func TestBlendStyle_DefaultColorsReplaced(t *testing.T) {
	cell := render.Cell{Char: 'A', Fg: render.ColorDefault, Bg: render.ColorDefault}
	wantFg := render.ColorIndexed | 1 // red
	wantBg := render.ColorIndexed | 4 // blue

	got := render.BlendStyle(cell, wantFg, wantBg, 0, 0, 0, 0, 0, 0)

	if got.Fg != wantFg {
		t.Errorf("Fg = %v, want %v", got.Fg, wantFg)
	}
	if got.Bg != wantBg {
		t.Errorf("Bg = %v, want %v", got.Bg, wantBg)
	}
	if got.Char != 'A' {
		t.Errorf("Char = %q, want 'A'", got.Char)
	}
}

// TestBlendStyle_NonDefaultNotOverridden verifies that cells with explicit
// (non-default) colours are not changed by BlendStyle.
func TestBlendStyle_NonDefaultNotOverridden(t *testing.T) {
	origFg := render.ColorIndexed | 2 // green
	origBg := render.ColorIndexed | 5 // magenta
	cell := render.Cell{Char: 'B', Fg: origFg, Bg: origBg}

	got := render.BlendStyle(cell, render.ColorIndexed|1, render.ColorIndexed|4, 0, 0, 0, 0, 0, 0)

	if got.Fg != origFg {
		t.Errorf("Fg = %v, want original %v", got.Fg, origFg)
	}
	if got.Bg != origBg {
		t.Errorf("Bg = %v, want original %v", got.Bg, origBg)
	}
}

// TestBlendStyle_DefaultDefaultUnchanged verifies that when the supplied
// defaults are themselves ColorDefault, no change is made.
func TestBlendStyle_DefaultDefaultUnchanged(t *testing.T) {
	cell := render.Cell{Char: 'C', Fg: render.ColorDefault, Bg: render.ColorDefault}

	got := render.BlendStyle(cell, render.ColorDefault, render.ColorDefault, 0, 0, 0, 0, 0, 0)

	if got.Fg != render.ColorDefault {
		t.Errorf("Fg = %v, want ColorDefault", got.Fg)
	}
	if got.Bg != render.ColorDefault {
		t.Errorf("Bg = %v, want ColorDefault", got.Bg)
	}
}

// TestCompose_WindowStyle_InactivePaneColored verifies that cells with
// ColorDefault fg/bg in an inactive pane receive the window-style colours.
func TestCompose_WindowStyle_InactivePaneColored(t *testing.T) {
	r := render.New(render.Config{
		Rows: 2,
		Cols: 4,
		Theme: render.Theme{
			WindowStyle:       "fg=red,bg=blue",
			WindowActiveStyle: "fg=green,bg=yellow",
		},
		ActivePaneID: 1, // pane 1 is active; pane 0 is inactive
	})

	// Pane 0 is inactive — should get window-style colours.
	cells := []render.Cell{
		{Char: 'A', Fg: render.ColorDefault, Bg: render.ColorDefault},
		{Char: 'B', Fg: render.ColorDefault, Bg: render.ColorDefault},
		{Char: 'C', Fg: render.ColorDefault, Bg: render.ColorDefault},
		{Char: 'D', Fg: render.ColorDefault, Bg: render.ColorDefault},
	}
	pane := &fakePane{
		bounds: render.Rect{X: 0, Y: 0, Width: 2, Height: 2},
		grid:   render.CellGrid{Rows: 2, Cols: 2, Cells: cells},
	}

	grid := r.Compose([]render.PanePlacement{{Pane: pane, Rect: pane.bounds, PaneIndex: 0}}, nil)

	wantFg := render.ColorIndexed | 1 // red
	wantBg := render.ColorIndexed | 4 // blue

	for row := 0; row < 2; row++ {
		for col := 0; col < 2; col++ {
			c := cellAt(grid, row, col)
			if c.Fg != wantFg {
				t.Errorf("cell(%d,%d).Fg = %v, want %v (window-style fg)", row, col, c.Fg, wantFg)
			}
			if c.Bg != wantBg {
				t.Errorf("cell(%d,%d).Bg = %v, want %v (window-style bg)", row, col, c.Bg, wantBg)
			}
		}
	}
}

// TestCompose_WindowActiveStyle_ActivePaneColored verifies that cells with
// ColorDefault fg/bg in the active pane receive window-active-style colours.
func TestCompose_WindowActiveStyle_ActivePaneColored(t *testing.T) {
	r := render.New(render.Config{
		Rows: 2,
		Cols: 2,
		Theme: render.Theme{
			WindowStyle:       "fg=red,bg=blue",
			WindowActiveStyle: "fg=green,bg=yellow",
		},
		ActivePaneID: 0, // pane 0 is active
	})

	cells := []render.Cell{
		{Char: 'A', Fg: render.ColorDefault, Bg: render.ColorDefault},
		{Char: 'B', Fg: render.ColorDefault, Bg: render.ColorDefault},
		{Char: 'C', Fg: render.ColorDefault, Bg: render.ColorDefault},
		{Char: 'D', Fg: render.ColorDefault, Bg: render.ColorDefault},
	}
	pane := &fakePane{
		bounds: render.Rect{X: 0, Y: 0, Width: 2, Height: 2},
		grid:   render.CellGrid{Rows: 2, Cols: 2, Cells: cells},
	}

	grid := r.Compose([]render.PanePlacement{{Pane: pane, Rect: pane.bounds, PaneIndex: 0}}, nil)

	wantFg := render.ColorIndexed | 2 // green
	wantBg := render.ColorIndexed | 3 // yellow

	for row := 0; row < 2; row++ {
		for col := 0; col < 2; col++ {
			c := cellAt(grid, row, col)
			if c.Fg != wantFg {
				t.Errorf("cell(%d,%d).Fg = %v, want %v (window-active-style fg)", row, col, c.Fg, wantFg)
			}
			if c.Bg != wantBg {
				t.Errorf("cell(%d,%d).Bg = %v, want %v (window-active-style bg)", row, col, c.Bg, wantBg)
			}
		}
	}
}

// TestCompose_WindowStyle_NonDefaultPreserved verifies that cells with
// explicit (non-default) colours are not overridden by window-style.
func TestCompose_WindowStyle_NonDefaultPreserved(t *testing.T) {
	r := render.New(render.Config{
		Rows: 1,
		Cols: 2,
		Theme: render.Theme{
			WindowStyle: "fg=red,bg=blue",
		},
		ActivePaneID: 1, // pane 0 is inactive
	})

	explicitFg := render.ColorIndexed | 7 // white — explicit, must not be overridden
	explicitBg := render.ColorIndexed | 0 // black — explicit, must not be overridden
	cells := []render.Cell{
		{Char: 'X', Fg: explicitFg, Bg: explicitBg},
		{Char: 'Y', Fg: render.ColorDefault, Bg: render.ColorDefault},
	}
	pane := &fakePane{
		bounds: render.Rect{X: 0, Y: 0, Width: 2, Height: 1},
		grid:   render.CellGrid{Rows: 1, Cols: 2, Cells: cells},
	}

	grid := r.Compose([]render.PanePlacement{{Pane: pane, Rect: pane.bounds, PaneIndex: 0}}, nil)

	// Cell 0: explicit colours must be preserved.
	c0 := cellAt(grid, 0, 0)
	if c0.Fg != explicitFg {
		t.Errorf("cell(0,0).Fg = %v, want %v (explicit fg must not be overridden)", c0.Fg, explicitFg)
	}
	if c0.Bg != explicitBg {
		t.Errorf("cell(0,0).Bg = %v, want %v (explicit bg must not be overridden)", c0.Bg, explicitBg)
	}

	// Cell 1: default colours should be replaced by window-style.
	c1 := cellAt(grid, 0, 1)
	if c1.Fg != render.ColorIndexed|1 {
		t.Errorf("cell(0,1).Fg = %v, want red (%v)", c1.Fg, render.ColorIndexed|1)
	}
	if c1.Bg != render.ColorIndexed|4 {
		t.Errorf("cell(0,1).Bg = %v, want blue (%v)", c1.Bg, render.ColorIndexed|4)
	}
}

// TestCompose_NilStatusNoReservedRow verifies that when Status is nil,
// all rows are available for panes.
func TestCompose_NilStatusNoReservedRow(t *testing.T) {
	r := render.New(render.Config{Rows: 2, Cols: 2, Status: nil, StatusRows: 1})

	pane := &fakePane{
		bounds: render.Rect{X: 0, Y: 0, Width: 2, Height: 2},
		grid:   makeGrid(2, 2, 'P'),
	}

	grid := r.Compose([]render.PanePlacement{{Pane: pane, Rect: pane.bounds}}, nil)

	// All rows should be filled by the pane since Status is nil.
	for row := 0; row < 2; row++ {
		if got := cellAt(grid, row, 0).Char; got != 'P' {
			t.Errorf("row %d = %q, want 'P'", row, got)
		}
	}
}
