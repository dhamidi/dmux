// Package menu implements `display-menu` — a small popup list of
// labeled items with callback-based activation.
//
// # Boundary
//
// Implements [modes.ClientOverlay]. The package imports only
// [github.com/dhamidi/dmux/internal/keys] and
// [github.com/dhamidi/dmux/internal/modes]; it has no dependency on
// any command-dispatch infrastructure.
//
// # MenuItem
//
// Each entry in the menu is described by a [MenuItem]:
//
//	type MenuItem struct {
//	    Label     string  // displayed text
//	    Mnemonic  rune    // optional single-key shortcut; 0 = none
//	    Separator bool    // render as a horizontal rule; not selectable
//	    Enabled   bool    // if false the item is shown but cannot be activated
//	    OnSelect  func()  // called on activation; may be nil
//	}
//
// When an item is activated (via Enter, mnemonic key, or mouse click)
// its OnSelect callback is invoked and the menu closes. The menu never
// dispatches commands itself — all behaviour is driven by the provided
// callback. This makes the package testable in isolation without any
// command queue or server.
//
// # Construction
//
//	m := menu.New(anchor, items)
//
// anchor is a [modes.Rect] whose X/Y supply the top-left corner of the
// menu in screen (client) coordinates. The menu self-sizes to the
// widest rendered label and its height equals the number of items.
//
// # Navigation
//
// Arrow Up/Down move the selection to the previous/next enabled,
// non-separator item (wrapping around). Enter activates the selection.
// Escape closes the menu without calling any callback. A letter key
// matching an item's Mnemonic activates that item directly.
//
// # Mouse
//
// Menus are commonly opened by right-clicking on the status line,
// window tabs, or pane borders. Motion events inside the menu update
// the hover highlight. A left-button click activates the item under
// the pointer. Events outside the bounding rectangle are passed through
// to the layer below.
//
// # Sizing
//
// The menu sizes itself to the widest rendered label plus a two-character
// prefix (the selection marker and a space). Position is anchored to the
// screen coordinate supplied at construction (typically the mouse-click
// location or the active pane's top-left corner).
//
// # Non-goals
//
// No sub-menus (yet). No scrolling — menus taller than the screen are
// truncated, matching tmux's current behavior.
package menu
