// Package status renders the dmux status line.
//
// In M1 the status line is one cell-row at the bottom (or top) of
// each attached client's screen. It shows session name, window list,
// and active-window indicator. It also displays transient error
// messages from CommandResult frames.
//
// In M5 it gains format-string expansion (`#{session_name}`),
// per-side composition (status-left, status-right), and styling
// directives (`#[fg=green]`). M1 hardcodes the layout; the package
// exists from M1 so the rendering and error-display paths are wired
// from day one.
//
// # M1 layout
//
//	[session-name] 0:shell* 1:vim 2:logs
//	    \  prefix \   \      \    \
//	     \         \   \      \    `--- inactive window
//	      \         \   \      `------ inactive window
//	       \         \   `------------ active window (suffix `*`)
//	        \         `--------------- window-index : window-name
//	         `-------------------------- session-name in brackets
//
// Style: dim background, bright text on the active window, dim text
// elsewhere. Profile-aware: 24-bit colour on all M1-supported
// terminals.
//
// # Error / message overlay
//
// When a CommandResult frame carries an error or a display-message
// command fires, the status line replaces its normal content with
// the message in an alert style for ~3 seconds, then reverts. This
// gives M1 a defined place for command errors to surface without
// requiring a separate notification system.
//
// # Interface
//
//	type View struct {
//	    Session    *session.Session
//	    Width      int
//	    Profile    termcaps.Profile
//	    Message    string        // empty for normal status
//	    Style      Style         // Normal | Alert | Info
//	}
//
//	Render(v View) []vt.Cell    // returns one cell-row of styled cells
//
// The server composes Render output with the focused pane's grid
// when building the per-client frame for termout. The status row is
// drawn at row 0 (top) or row sy-1 (bottom) per the
// `status-position` option.
//
// # Where status rendering happens
//
// On the server's main goroutine. The status content is small (one
// row) and its inputs (session/window state, profile) are owned by
// the main goroutine. No need to push it down to a pane goroutine.
//
// # Composition with pane content
//
// termout receives a composite View from server containing both the
// pane Grid and the status cells. The diff renderer treats the
// status row as just another row in the frame; differences in the
// status row produce minimal updates the same way pane-content
// differences do.
//
// # Update triggers
//
// The status line re-renders when:
//
//   - The session's current window changes (window switch).
//   - A window's name changes (rename-window).
//   - A new window is created or destroyed.
//   - An error or message arrives.
//   - The 3-second message timer expires.
//
// All triggers are server-side events; the main loop calls
// status.Render and pushes a fresh composite Output frame to the
// affected client.
//
// In M1 the status line does NOT re-render on a periodic timer.
// M5's clock display and other format-string time variables will
// add a 1-second tick.
//
// # Scope boundary
//
//   - No format-string parsing (M5).
//   - No interactive elements (no clicking on a window name to
//     switch to it; M3 with mouse).
//   - No menus invoked from status (post-M5).
//
// # Corresponding tmux code
//
// tmux's status.c plus parts of format.c (which we deliberately
// avoid in M1 by hardcoding layout). The rendering loop is
// equivalent to tmux's status_redraw with the format expansion
// short-circuited to literal text.
package status
