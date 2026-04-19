// Package tree implements the session/window/pane chooser
// (`prefix s`, `choose-tree`, `choose-session`, `choose-window`).
//
// # Boundary
//
// Implements modes.PaneMode. Takes a *session.Server snapshot at
// construction and renders a collapsible tree:
//
//	session-a
//	  0: editor*
//	  1: shell
//	session-b
//	  0: logs
//
// Supports arrow-key navigation, /-search, Enter to select, Esc or
// q to cancel, x to kill, and the usual vim/emacs movement keys.
//
// On selection, returns a modes.Outcome carrying the command to run
// (typically `switch-client -t <target>` or `select-window -t`),
// which the server loop enqueues.
//
// # Preview
//
// The right half of the mode's area previews the currently-highlighted
// target using the same pane-snapshot mechanism render uses, so users
// can see what they're switching to before committing.
//
// # In isolation
//
// Construct a fake Server with several Sessions and Windows, render
// the tree to a cell grid, assert on the output. No client or real
// terminal required.
//
// # Non-goals
//
// Does not mutate server state. It only emits commands as Outcomes.
// This keeps the mode reusable — `choose-buffer`, `choose-client`,
// and `choose-customize` can be implemented in this package as
// alternative Constructors over the same tree infrastructure.
package tree
