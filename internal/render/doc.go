// Package render composes panes, borders, status lines, and client
// overlays into a single cell grid ready for term.Flush.
//
// # Exported interfaces
//
// [Renderer] is the primary interface callers depend on:
//
//	type Renderer interface {
//	    Compose(panes []PanePlacement, overlays []Overlay) CellGrid
//	}
//
// Callers (server, client) hold a Renderer and never import the
// concrete renderer type. New(cfg Config) returns a Renderer.
//
// # Accepted interfaces
//
// All dependencies are accepted as narrow interfaces so that callers
// can test composition logic with fakes and real terminal machinery is
// never required.
//
// [Pane] — the render package's view of a terminal pane:
//
//	type Pane interface {
//	    Bounds() Rect          // screen rectangle this pane occupies
//	    Snapshot() CellGrid    // immutable snapshot of visible cells
//	}
//
// The concrete pane.Pane satisfies this interface via a thin adapter
// that converts pane.CellGrid to render.CellGrid.
//
// [StatusLine] — the render package's view of a status renderer:
//
//	type StatusLine interface {
//	    Render(width int) []Cell   // one row of exactly width cells
//	}
//
// [Overlay] — the drawing portion of modes.ClientOverlay:
//
//	type Overlay interface {
//	    Rect() Rect                        // bounding rectangle
//	    Render(dst []Cell)                 // fill dst (len=Width*Height)
//	}
//
// Non-drawing overlay methods (Key, Mouse, CaptureFocus, Close) are
// intentionally omitted; the server loop handles event routing.
//
// # Configuration
//
// [Config] holds all constructor inputs:
//
//	type Config struct {
//	    Rows, Cols  int         // output grid dimensions
//	    Status      StatusLine  // nil disables the status line
//	    StatusRows  int         // rows reserved for status (typically 0–1)
//	    Theme       Theme       // border character and colours
//	}
//
// # Composition order
//
// Layers are applied bottom-up:
//
//  1. Background fill (all cells set to ' ')
//  2. Pane snapshots in PanePlacement order; later placements overwrite earlier.
//     Cells outside the pane's Rect are not touched.
//     Zero-rune cells in a snapshot are normalised to ' '.
//     Pane output is restricted to rows [0, Rows−StatusRows).
//  3. Status line cells written into the reserved bottom rows.
//  4. Overlays applied in order; each overlay's Rect determines its region.
//
// # Dirty tracking
//
// Per-row dirty tracking is a caller responsibility: callers may skip
// passing panes whose rows have not changed. render.Compose always
// writes all pane and overlay cells it receives and does not maintain
// internal dirty state.
//
// # Data types
//
// [Cell] and [CellGrid] are defined in this package; callers that bridge
// from pane.CellGrid must convert element-by-element (both are
// struct{Char rune} / row-major grids of the same shape).
//
// [Rect] is a type alias for layout.Rect so callers need not import
// layout directly:
//
//	type Rect = layout.Rect  // {X, Y, Width, Height int}
//
// # In isolation
//
// Tests use fake implementations of Pane, StatusLine, and Overlay
// that return canned CellGrid data. No real PTY, terminal emulator,
// or status renderer is required to test composition logic.
//
// # Non-goals
//
// Not a terminal driver. The returned CellGrid is data — writing it
// to the tty is the job of term.Flush. Not a status renderer — the
// status package produces []Cell; render just places them.
package render
