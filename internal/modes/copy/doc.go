// Package copy implements copy-mode: a vi/emacs-style editor over a
// pane's scrollback.
//
// # Boundary
//
// Implements modes.PaneMode. Takes a *pane.Pane at construction,
// reads its render state and scrollback via libghostty's Formatter
// and PointFromViewport / ViewportFromPoint APIs, and maintains:
//
//   - cursor position (independent of the pane's shell cursor)
//   - optional selection anchor
//   - search state (last query, direction, matches)
//   - a key table name ("copy-mode-vi" or "copy-mode") — the client's
//     KeyTable is set to this while the mode is active
//
// # Operations
//
// All copy-mode commands are normal gomux commands, registered in
// package command/builtin under names like `send -X cursor-up`,
// `send -X begin-selection`, `send -X copy-selection`, etc. This
// package provides the state and rendering; commands dispatched
// while the client's KeyTable is "copy-mode-vi" operate on it.
//
// # Rendering
//
// Render draws the pane's scrollback region into the pane's rectangle,
// overlaying the copy-mode cursor and any active selection. The
// underlying pane's libghostty Terminal is never mutated.
//
// # In isolation
//
// Testable by feeding canned VT bytes into a pane.Pane, entering
// copy-mode, driving Key calls, and asserting on selection / search
// / copy output.
//
// # Non-goals
//
// No clipboard integration — that happens in the `copy-selection`
// command, which emits OSC 52 via the client or pipes to an external
// program.
package copy
