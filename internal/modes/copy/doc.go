// Package copy implements copy-mode: a vi/emacs-style editor over a
// pane's scrollback.
//
// # Module boundary
//
// The package defines the [Scrollback] interface so that copy-mode has
// no compile-time dependency on internal/pane or any concrete terminal
// type. Callers pass in a [Scrollback] at construction; tests use a
// stub implementation.
//
//	type Scrollback interface {
//	    Lines() []Line   // all buffered rows, oldest first
//	    Width() int      // terminal width in columns
//	    Height() int     // terminal height in rows (visible viewport)
//	}
//
// [Line] is defined as []modes.Cell, one element per terminal column.
//
// # PaneMode contract
//
// [Mode] implements [modes.PaneMode]:
//
//	Render(dst modes.Canvas)            — draws the scrollback viewport
//	Key(k keys.Key) modes.Outcome       — maps keys to Command calls
//	Mouse(ev keys.MouseEvent) modes.Outcome — no-op; returns Consumed
//	Close()                             — no-op; holds no resources
//
// # State
//
// A Mode maintains:
//
//   - cursor position (row and column, independent of the pane's shell cursor)
//   - optional selection anchor (set by begin-selection, cleared by copy-selection
//     or clear-selection)
//   - search state (most recent query and direction)
//   - a view offset (first line of the scrollback visible in the viewport)
//
// # Command dispatch
//
// All copy-mode operations are driven through [Mode.Command], which
// accepts the same names used by tmux's `send -X` mechanism.
// Recognised commands:
//
//	cursor-up, cursor-down, cursor-left, cursor-right
//	start-of-line, end-of-line
//	page-up, page-down
//	history-top, history-bottom
//	begin-selection, clear-selection, copy-selection
//	search-again, search-reverse
//	cancel
//
// [Mode.Key] maps raw [keys.Key] events to Command calls. [Mode.SetSearch]
// sets the search query/direction and immediately jumps to the first match.
//
// copy-selection returns a [modes.Command] outcome whose Cmd field is a
// [CopyCommand]{Text: …}. The host is responsible for transmitting the
// text to the clipboard or client (for example via OSC 52).
//
// # Rendering
//
// Render draws the scrollback lines visible in the current viewport onto
// dst. The viewport is adjusted automatically so that the cursor is always
// on-screen. The underlying scrollback is never mutated.
//
// # Non-goals
//
// No clipboard integration — that is the responsibility of the host that
// receives the [CopyCommand] outcome from copy-selection.
package copy
