package render

import (
	"strconv"

	"github.com/dhamidi/dmux/internal/format"
	"github.com/dhamidi/dmux/internal/layout"
)

// SGR attribute flags for Cell.Attrs.
const (
	AttrBold            uint16 = 1 << 0
	AttrReverse         uint16 = 1 << 1
	AttrUnderline       uint16 = 1 << 2
	AttrBlink           uint16 = 1 << 3
	AttrDim             uint16 = 1 << 4
	AttrItalics         uint16 = 1 << 5
	AttrOverline        uint16 = 1 << 6
	AttrStrikethrough   uint16 = 1 << 7
	AttrDoubleUnderline uint16 = 1 << 8
	AttrCurlyUnderline  uint16 = 1 << 9
)

// Color is an 8-bit terminal color index (0–255) or one of the sentinel
// values ColorDefault (terminal default) and ColorRGB (use R,G,B fields).
type Color uint16

const (
	ColorDefault Color = 0      // terminal's default color
	ColorIndexed  Color = 0x100 // sentinel: use low byte as 256-color index
	ColorRGB      Color = 0x200 // sentinel: use R,G,B fields
)

// Cell is a single display cell in a composed frame with styling.
type Cell struct {
	Char  rune  // displayed character; 0 means empty (treated as space)
	Fg    Color // foreground color; ColorDefault means inherit
	Bg    Color // background color; ColorDefault means inherit
	Attrs uint16 // bitmask of Attr* constants
	// FgR, FgG, FgB are meaningful only when Fg == ColorRGB.
	FgR, FgG, FgB uint8
	// BgR, BgG, BgB are meaningful only when Bg == ColorRGB.
	BgR, BgG, BgB uint8
}

// CellGrid is a rectangular grid of [Cell] values in row-major order.
// Cells[row*Cols+col] is the cell at (col, row), origin at top-left.
type CellGrid struct {
	Rows  int
	Cols  int
	Cells []Cell
}

// Rect describes the position and size of a region in cell coordinates.
// It is an alias for [layout.Rect] so callers need not import layout.
type Rect = layout.Rect

// Pane is the narrow interface [Renderer] requires of a terminal pane.
// The render package never imports the concrete pane type; callers
// provide an adapter that satisfies this interface.
type Pane interface {
	// Bounds returns the screen rectangle this pane occupies.
	Bounds() Rect
	// Snapshot returns an immutable snapshot of the pane's visible cells.
	Snapshot() CellGrid
}

// StatusLine is the narrow interface [Renderer] requires of a status renderer.
// The concrete implementation lives in package status; render only calls Render.
type StatusLine interface {
	// Render returns a slice of exactly width cells representing one row
	// of the status line. A nil return means no status line.
	Render(width int) []Cell
}

// Overlay is the narrow interface [Renderer] requires of a client overlay.
// It matches the drawing portion of modes.ClientOverlay; non-drawing
// methods (key/mouse handling, CaptureFocus, Close) are not needed here.
type Overlay interface {
	// Rect returns the overlay's bounding rectangle in screen coordinates.
	Rect() Rect
	// Render fills dst (length Rect().Width*Rect().Height) with the
	// overlay's cells, in row-major order.
	Render(dst []Cell)
}

// PanePlacement pairs a [Pane] with the screen rectangle it occupies.
// The Rect field takes precedence over Pane.Bounds() during composition
// so that zoom/override rects can be applied without mutating the pane.
type PanePlacement struct {
	Pane Pane
	Rect Rect
	// SynchronizedPanes, when true, causes the pane border to be rendered
	// with a distinct marker (*) to indicate that synchronize-panes is active.
	SynchronizedPanes bool
	// PaneIndex is the zero-based index of this pane, used when expanding
	// the pane-border-format string (#{pane_index}).
	PaneIndex int
}

// Theme configures visual aspects of the composed frame.
type Theme struct {
	// BorderLines is the pane-border-lines option value ("single", "double",
	// "heavy", "simple", or "padded"). An empty string disables border drawing.
	BorderLines string

	// PaneBorderStatus controls whether and where a label is shown on pane
	// borders. Accepted values: "off" (default, no label), "top" (label on the
	// horizontal border above each pane), "bottom" (label on the horizontal
	// border below each pane).
	PaneBorderStatus string

	// PaneBorderFormat is the format string expanded for each pane's border
	// label. Defaults to "#{pane_index}" when empty.
	PaneBorderFormat string
}

// Config holds all dependencies for a [Renderer].
type Config struct {
	// Rows is the total height of the output grid in cells.
	Rows int
	// Cols is the total width of the output grid in cells.
	Cols int

	// Status provides status-line cells. Nil means no status line.
	Status StatusLine
	// StatusRows is the number of rows reserved for the status line.
	// Typically 0 (disabled) or 1. Ignored when Status is nil.
	StatusRows int

	// Theme controls border and inactive-pane rendering.
	Theme Theme
}

// Renderer composes pane snapshots and overlays into a single [CellGrid].
//
// Callers (server, client) depend on this interface; the concrete type is
// returned by [New] and never exported directly.
type Renderer interface {
	// Compose blits pane snapshots into a fresh grid, renders the status
	// line (if configured), then applies overlays in order.
	//
	// Panes are rendered in the order given; later placements overwrite
	// earlier ones in areas of overlap. Overlays are applied on top of
	// the fully composed pane layer, also in order.
	Compose(panes []PanePlacement, overlays []Overlay) CellGrid
}

// renderer is the concrete implementation of [Renderer].
type renderer struct {
	cfg Config
}

// New creates a [Renderer] from cfg.
func New(cfg Config) Renderer {
	return &renderer{cfg: cfg}
}

// Compose implements [Renderer].
func (r *renderer) Compose(panes []PanePlacement, overlays []Overlay) CellGrid {
	rows := r.cfg.Rows
	cols := r.cfg.Cols

	grid := CellGrid{
		Rows:  rows,
		Cols:  cols,
		Cells: make([]Cell, rows*cols),
	}

	// Fill background with spaces.
	for i := range grid.Cells {
		grid.Cells[i] = Cell{Char: ' '}
	}

	// Compute the row range available to panes (reserving status rows).
	paneRows := rows
	statusRow := -1
	if r.cfg.Status != nil && r.cfg.StatusRows > 0 {
		paneRows = rows - r.cfg.StatusRows
		statusRow = paneRows
	}

	// Blit each pane snapshot into the grid.
	for _, pp := range panes {
		snap := pp.Pane.Snapshot()
		rect := pp.Rect

		for row := 0; row < rect.Height && row < snap.Rows; row++ {
			dstRow := rect.Y + row
			if dstRow < 0 || dstRow >= paneRows {
				continue
			}
			for col := 0; col < rect.Width && col < snap.Cols; col++ {
				dstCol := rect.X + col
				if dstCol < 0 || dstCol >= cols {
					continue
				}
				cell := snap.Cells[row*snap.Cols+col]
				if cell.Char == 0 {
					cell.Char = ' '
				}
				grid.Cells[dstRow*cols+dstCol] = cell
			}
		}
	}

	// Draw pane borders when BorderLines is configured.
	if r.cfg.Theme.BorderLines != "" {
		r.drawBorders(&grid, panes, paneRows, cols)
	}

	// Overlay border labels when pane-border-status is "top" or "bottom".
	r.drawBorderLabels(&grid, panes, paneRows, cols)

	// Draw synchronize-panes border markers (*) at the right and bottom edges
	// of each pane that has SynchronizedPanes set, using a yellow colour.
	syncBorderCell := Cell{Char: '*', Fg: ColorIndexed | 3}
	for _, pp := range panes {
		if !pp.SynchronizedPanes {
			continue
		}
		rect := pp.Rect
		// Right edge column.
		for row := rect.Y; row < rect.Y+rect.Height && row < paneRows; row++ {
			col := rect.X + rect.Width - 1
			if col >= 0 && col < cols {
				grid.Cells[row*cols+col] = syncBorderCell
			}
		}
		// Bottom edge row.
		row := rect.Y + rect.Height - 1
		if row >= 0 && row < paneRows {
			for col := rect.X; col < rect.X+rect.Width && col < cols; col++ {
				grid.Cells[row*cols+col] = syncBorderCell
			}
		}
	}

	// Render the status line into the reserved rows.
	if statusRow >= 0 {
		statusCells := r.cfg.Status.Render(cols)
		for col, cell := range statusCells {
			if col >= cols {
				break
			}
			grid.Cells[statusRow*cols+col] = cell
		}
	}

	// Apply overlays on top.
	for _, ov := range overlays {
		rect := ov.Rect()
		if rect.Width <= 0 || rect.Height <= 0 {
			continue
		}
		dst := make([]Cell, rect.Width*rect.Height)
		ov.Render(dst)
		for row := 0; row < rect.Height; row++ {
			dstRow := rect.Y + row
			if dstRow < 0 || dstRow >= rows {
				continue
			}
			for col := 0; col < rect.Width; col++ {
				dstCol := rect.X + col
				if dstCol < 0 || dstCol >= cols {
					continue
				}
				grid.Cells[dstRow*cols+dstCol] = dst[row*rect.Width+col]
			}
		}
	}

	return grid
}

// drawBorderLabels overlays a format-expanded label on the top or bottom
// horizontal border of each pane, as configured by Theme.PaneBorderStatus.
//
// "top" places the label on the horizontal border row immediately above the
// pane (rect.Y - 1); "bottom" places it on the pane's own bottom border row
// (rect.Y + rect.Height - 1). The label is centered in the interior of the
// border (pane width minus the two vertical border columns on each side) and
// padded with the BorderSet horizontal character.
func (r *renderer) drawBorderLabels(grid *CellGrid, panes []PanePlacement, paneRows, cols int) {
	status := r.cfg.Theme.PaneBorderStatus
	if status != "top" && status != "bottom" {
		return
	}

	fmtStr := r.cfg.Theme.PaneBorderFormat
	if fmtStr == "" {
		fmtStr = "#{pane_index}"
	}

	bs := BorderSetForName(r.cfg.Theme.BorderLines)

	for _, pp := range panes {
		rect := pp.Rect

		var targetRow int
		if status == "top" {
			targetRow = rect.Y - 1
		} else {
			targetRow = rect.Y + rect.Height - 1
		}

		if targetRow < 0 || targetRow >= paneRows {
			continue
		}

		ctx := format.MapContext{"pane_index": strconv.Itoa(pp.PaneIndex)}
		label, _ := format.Expand(fmtStr, ctx)

		// Available interior width: exclude the vertical border columns on each side.
		maxWidth := rect.Width - 2
		if maxWidth <= 0 {
			continue
		}

		// Truncate label to fit.
		runes := []rune(label)
		if len(runes) > maxWidth {
			runes = runes[:maxWidth]
		}
		labelLen := len(runes)

		// Center the label within maxWidth.
		totalPad := maxWidth - labelLen
		leftPad := totalPad / 2

		startCol := rect.X + 1

		// Left horizontal padding.
		for j := 0; j < leftPad; j++ {
			col := startCol + j
			if col >= 0 && col < cols {
				grid.Cells[targetRow*cols+col] = Cell{Char: bs.Horizontal}
			}
		}

		// Label characters.
		for j, ch := range runes {
			col := startCol + leftPad + j
			if col >= 0 && col < cols {
				grid.Cells[targetRow*cols+col] = Cell{Char: ch}
			}
		}

		// Right horizontal padding.
		rightStart := startCol + leftPad + labelLen
		rightEnd := rect.X + rect.Width - 1 // exclusive: stop before vertical border
		for col := rightStart; col < rightEnd && col < cols; col++ {
			grid.Cells[targetRow*cols+col] = Cell{Char: bs.Horizontal}
		}
	}
}

// drawBorders draws pane border characters into grid using the BorderSet
// determined by r.cfg.Theme.BorderLines.
//
// For each pane placement, the rightmost column of its rect is treated as a
// vertical border and the bottom row as a horizontal border. At intersections
// the appropriate junction character (corner, T, or cross) is selected.
func (r *renderer) drawBorders(grid *CellGrid, panes []PanePlacement, paneRows, cols int) {
	type borderInfo struct {
		isVert  bool
		isHoriz bool
	}

	borderGrid := make([]borderInfo, paneRows*cols)

	for _, pp := range panes {
		rect := pp.Rect

		// Right column → vertical border segment.
		bc := rect.X + rect.Width - 1
		if bc >= 0 && bc < cols {
			for row := rect.Y; row < rect.Y+rect.Height && row < paneRows; row++ {
				if row >= 0 {
					borderGrid[row*cols+bc].isVert = true
				}
			}
		}

		// Bottom row → horizontal border segment.
		br := rect.Y + rect.Height - 1
		if br >= 0 && br < paneRows {
			for col := rect.X; col < rect.X+rect.Width && col < cols; col++ {
				if col >= 0 {
					borderGrid[br*cols+col].isHoriz = true
				}
			}
		}
	}

	isVert := func(row, col int) bool {
		if row < 0 || row >= paneRows || col < 0 || col >= cols {
			return false
		}
		return borderGrid[row*cols+col].isVert
	}
	isHoriz := func(row, col int) bool {
		if row < 0 || row >= paneRows || col < 0 || col >= cols {
			return false
		}
		return borderGrid[row*cols+col].isHoriz
	}

	bs := BorderSetForName(r.cfg.Theme.BorderLines)

	for row := 0; row < paneRows; row++ {
		for col := 0; col < cols; col++ {
			b := borderGrid[row*cols+col]
			if !b.isVert && !b.isHoriz {
				continue
			}

			var ch rune
			if b.isVert && b.isHoriz {
				hasTop := isVert(row-1, col)
				hasBottom := isVert(row+1, col)
				hasLeft := isHoriz(row, col-1)
				hasRight := isHoriz(row, col+1)
				ch = bs.junctionChar(hasTop, hasBottom, hasLeft, hasRight)
			} else if b.isVert {
				ch = bs.Vertical
			} else {
				ch = bs.Horizontal
			}

			if ch != 0 {
				grid.Cells[row*cols+col] = Cell{Char: ch}
			}
		}
	}
}
