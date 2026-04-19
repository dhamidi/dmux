// Package popup implements `display-popup` — a floating terminal
// window rendered over the current view.
//
// # Boundary
//
// Implements modes.ClientOverlay. Constructed with a command to run
// (or no command, for a blank shell), a size, and a position. Under
// the hood it owns a *pane.Pane running in a PTY sized to the popup,
// with no awareness that it's floating.
//
// # Why this composes
//
// Because pane.Pane is standalone (Tier 1) and doesn't know anything
// about windows, layouts, or sessions, the same code that runs a
// normal tiled pane runs inside a popup. The popup just wraps it
// with borders in client-space and routes keys to it while focused.
//
// When the popup closes, the pane is killed. When the command inside
// exits, the popup auto-closes (if `-E` was passed).
//
// # Border and title
//
// Drawn here, outside the wrapped Pane's rectangle. Theme comes from
// options (`popup-border-style`, `popup-border-lines`).
//
// # In isolation
//
// Testable by constructing a popup with a trivial command like `echo
// hi`, pumping the overlay's frame updates, and asserting on the
// rendered cells including borders.
//
// # Non-goals
//
// Not a tabbed multi-popup manager — one popup per client at a time.
// Stacking multiple popups is not a planned feature.
package popup
