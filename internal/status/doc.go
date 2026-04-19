// Package status renders the status line(s) into cells.
//
// # Boundary
//
// Render(client *session.Client, server *session.Server) []Line
// produces zero or more Lines of cells. Each Line is the width of the
// client and a single row tall. Lines are positioned by the caller
// (package render) according to the status-position option.
//
// A status line is configured by three format strings:
//
//   - status-left
//   - status-format-0, status-format-1, ... (numbered per line)
//   - status-right
//
// Each is expanded against a session.Context using package format.
// The expanded strings may contain embedded #[fg=color,bg=color]
// style markers that this package parses into cell attributes — tmux
// calls these "style ranges."
//
// # Style ranges and clicks
//
// A Line also carries a slice of Range entries mapping cell columns
// back to a command the status line binds to that region, so
// "click on the window tab to select-window" works. render stashes
// these; the server loop translates mouse events into command
// dispatches.
//
// # In isolation
//
// Renderable against a mocked session.Context. Golden-file tests
// verify particular format strings produce the right cells without
// ever booting a real server or pane.
//
// # Non-goals
//
// Not drawn here. render.Compose places the Lines. Not evaluated on
// a timer here; the server loop calls Render on its redraw cadence.
package status
