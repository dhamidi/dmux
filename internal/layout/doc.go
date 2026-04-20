// Package layout is the binary tree that describes how panes tile
// inside a window.
//
// # Boundary
//
// A Tree is a recursive structure: a Node is either a Leaf (one pane,
// identified by opaque LeafID) or a Split (horizontal or vertical)
// containing a list of child Nodes with relative sizes. The package
// operates purely on LeafIDs — it does not import package pane and
// does not know what a pane is.
//
// LeafID is a plain int type; callers map LeafIDs to their concrete pane
// objects. The layout package has zero imports of other internal/* packages
// and can be compiled, tested, and reasoned about in complete isolation.
//
// # Types
//
//	type LeafID    int                 // opaque identifier for a leaf pane
//	type Direction int                 // Horizontal or Vertical
//	type Edge      int                 // EdgeTop, EdgeBottom, EdgeLeft, EdgeRight
//	type Rect      struct{ X, Y, Width, Height int }
//	type Preset    int                 // PresetEvenHorizontal, PresetEvenVertical, …
//	type BorderID  struct{ PaneID LeafID; Edge Edge }
//
// # Public surface
//
//	New(cols, rows int, first LeafID) *Tree
//	(*Tree).Split(leaf LeafID, dir Direction) (new LeafID)
//	(*Tree).Close(leaf LeafID)
//	(*Tree).Resize(cols, rows int)
//	(*Tree).MoveBorder(leaf LeafID, edge Edge, delta int)
//	(*Tree).Rect(leaf LeafID) Rect
//	(*Tree).Leaves() iter.Seq[LeafID]
//	(*Tree).ApplyPreset(p Preset)           // even-horiz, tiled, etc.
//	(*Tree).Marshal() string                // tmux-compatible format
//	Unmarshal(s string) (*Tree, error)
//	(*Tree).Zoom(leaf LeafID)               // temporarily maximize
//	(*Tree).Unzoom()
//	PaneAt(t *Tree, col, row int) (LeafID, bool)
//	BorderAt(t *Tree, col, row int) (*BorderID, bool)
//
// The Marshal format matches tmux's "e6d4,80x24,0,0{...}" so existing
// layout strings work and external tools can read them.
//
// # Zoom
//
// Zoom is a tree attribute, not a mode. When zoomed, Rect(leaf) returns
// the full window rect for the zoomed leaf and a zero rect for others.
// This is how tmux's `resize-pane -Z` works.
//
// # In isolation
//
// Testable without any panes existing. A visualization test renders
// layouts to SVG for inspection. A "layout lint" standalone could
// validate a user-supplied layout string.
//
// # Non-goals
//
// No rendering. No knowledge of pane contents. No focus tracking
// (that's session). No status line (that's not in the window at all).
package layout
