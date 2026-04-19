// Package menu implements `display-menu` — a small popup list of
// labeled items bound to commands.
//
// # Boundary
//
// Implements modes.ClientOverlay. Constructed with a []Item:
//
//	type Item struct {
//	    Label    string        // may contain #{...} format markers
//	    Mnemonic rune          // optional single-key shortcut
//	    Command  string        // gomux command source to run
//	    Enabled  bool
//	    Separator bool         // render as a horizontal rule
//	}
//
// Navigation with arrow keys, Enter to activate, Esc to cancel,
// letter keys activate their mnemonic. Activation returns an Outcome
// carrying the command source to parse and enqueue.
//
// # Mouse
//
// Menus are commonly driven by right-clicking on the status line,
// window tabs, or pane borders. This package handles hover
// highlighting and click activation when mouse events are routed to it.
//
// # Sizing
//
// The menu sizes itself to the widest rendered label. Position is
// anchored to a screen coordinate supplied at construction (typically
// the mouse-click location or the active pane's top-left).
//
// # In isolation
//
// Testable by constructing a menu with static items, driving Key /
// Mouse calls, asserting on the emitted command source.
//
// # Non-goals
//
// No sub-menus (yet). No scrolling — menus taller than the screen
// are truncated, matching tmux's current behavior.
package menu
