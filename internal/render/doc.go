// Package render composes panes, borders, status lines, and client
// overlays into a single cell grid ready for term.Flush.
//
// # Boundary
//
// Compose(in Input) Frame, where Input is:
//
//	type Input struct {
//	    Size       Size              // client's terminal size
//	    Layout     *layout.Tree      // active window's layout
//	    Panes      Snapshotter       // pane snapshots by LeafID
//	    Status     []status.Segment  // rendered status line(s)
//	    Overlays   []ClientOverlay   // popups, menus, display-panes, etc.
//	    Theme      Theme             // border colors, inactive dimming
//	}
//
// Snapshotter is an interface with Snapshot(LeafID, *RenderState) — so
// tests can use fake panes, and render doesn't import pane. The Frame
// is a grid of cells plus the desired cursor state.
//
// # Layers
//
// Composed bottom-up:
//
//  1. Window background
//  2. Each pane rectangle from layout.Rect(), populated from its
//     render state
//  3. Pane borders, with the active pane highlighted
//  4. Status line(s) at configured position
//  5. ClientOverlays (popups, menus, display-panes numerals), in order
//
// # Dirty tracking
//
// Compose uses per-row dirty flags from libghostty render states and
// layout changes to avoid re-composing unchanged regions. term.Flush
// then does its own diff to the real terminal.
//
// # In isolation
//
// Testable with a mock Snapshotter that returns canned cell grids.
// Golden-file tests assert the composed frame for fixed inputs.
//
// # Non-goals
//
// Not a terminal driver. The Frame is data — actually writing to the
// tty is term.Flush(Frame). Not a status renderer — status produces
// its own cells; render just places them.
package render
