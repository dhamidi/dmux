package render

import "github.com/dhamidi/dmux/internal/layout"

// SGR attribute flags for Cell.Attrs.
const (
	AttrBold      uint8 = 1 << 0
	AttrReverse   uint8 = 1 << 1
	AttrUnderline uint8 = 1 << 2
	AttrBlink     uint8 = 1 << 3
	AttrDim       uint8 = 1 << 4
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
	Attrs uint8 // bitmask of Attr* constants
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
}

// Theme configures visual aspects of the composed frame.
type Theme struct {
	// BorderChar is the rune drawn for pane-border cells.
	// Zero defaults to a space (no visible border).
	BorderChar rune
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
