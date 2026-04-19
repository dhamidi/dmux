// Package displaypanes implements `display-panes` (default: prefix q)
// — a transient overlay that draws a big numeral on each pane and
// waits for a digit key to select one.
//
// # Boundary
//
// Implements modes.ClientOverlay. Constructed with the active window's
// layout and the indices of its panes. Registers a timer with the
// server loop (via a callback injected at construction) that auto-
// dismisses after display-panes-time milliseconds.
//
// On a digit keystroke, returns an Outcome carrying `select-pane -t
// :.<n>` for the server to enqueue. On timeout or Esc, closes with
// no command.
//
// # Rendering
//
// Big-numeral glyphs are drawn with box-drawing characters across
// multiple cells, centered in each pane's rectangle. Color comes
// from `display-panes-colour` / `display-panes-active-colour`
// options.
//
// # In isolation
//
// Testable by constructing with a mock layout and mock timer, driving
// keystrokes, asserting on the output commands.
//
// # Non-goals
//
// Does not itself maintain the timer — it asks for one via a callback
// so the server loop owns all real time. Keeping time out of this
// package makes it trivially testable.
package displaypanes
