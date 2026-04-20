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
