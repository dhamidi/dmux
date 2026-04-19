// Package render composes panes, borders, status lines, and client
// overlays into a single cell grid ready for term.Flush.
//
// # Boundary
//
// Compose(in Input) Frame, where every collaborator is an interface so
// no Tier 1 package needs to be imported just to render:
//
//	type Input struct {
//	    Size     Size           // client's terminal size
//	    Tiling   Tiling         // interface; *layout.Tree satisfies it
//	    Panes    Snapshotter    // interface; pane snapshots by LeafID
//	    Status   []Line         // pre-rendered status rows from caller
//	    Overlays []ClientOverlay
//	    Theme    Theme
//	}
//
//	type Tiling interface {
//	    Rect(leaf LeafID) Rect
//	    Leaves() iter.Seq[LeafID]
//	    Active() LeafID
//	}
//
//	type Snapshotter interface {
//	    Snapshot(leaf LeafID, rs *RenderState)
//	}
//
// Tests can supply a struct-literal Tiling and a fake Snapshotter; render
// imports neither layout nor pane. LeafID is a type alias re-exported
// here to avoid importing layout for the type. Status lines are passed
// in as already-rendered cell rows so render does not import status.
// The Frame is a grid of cells plus the desired cursor state.
//
// # I/O surfaces
//
// None. Compose is a pure function: data in, Frame out. Writing the
// Frame to a real terminal is term.Flush's job.
//
// # Layers
//
// Composed bottom-up:
//
//  1. Window background
//  2. Each pane rectangle from Tiling.Rect(), populated from its
//     render state via Snapshotter.Snapshot()
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
