// Package termout renders a pane's terminal state into bytes for a
// specific client's real terminal.
//
// # Inputs
//
//   - A vt.Grid snapshot: text cells with attributes, fg/bg as 24-bit
//     RGB, optional hyperlink IDs, optional graphics placement refs.
//   - A vt.Cursor: position, shape, visibility.
//   - A vt.Modes snapshot: alt-screen state, etc.
//   - A termcaps.Profile for the target client.
//   - The previous frame sent to that client (for diffing).
//
// # Outputs
//
//   - Bytes to place in an Output frame: SGR runs, cursor motions,
//     OSC 8 hyperlink wrappers, APC G kitty-graphics commands (Ghostty
//     only), and DCS sixel blobs (all three targets).
//
// # Strategy
//
//	Text:        diff against previous frame, emit minimal update.
//	Color:       24-bit SGR always (all three targets support it).
//	Hyperlinks:  OSC 8 always (all three targets support it).
//	Graphics:    kitty passthrough to Ghostty;
//	             sixel passthrough to all three targets;
//	             no cross-format conversion.
//
// # Multi-pane graphics
//
// When a pane emits kitty-graphics placements and the attached client
// is Ghostty, termout rewrites image IDs per pane to prevent collision
// on the real terminal and clips placement rectangles to the pane's
// screen area. This is deferred-but-nontrivial work that belongs here,
// not in internal/vt.
//
// # Capability-probe honesty
//
// When a pane's app sends a DA/DSR probe ("do you support kitty
// graphics?"), the server consults the attached client's profile and
// answers truthfully. That response routing is coordinated by
// internal/pane but is called out here for context: termout never
// silently drops graphics that the app believes will render.
//
// # Interface
//
//	NewRenderer(termcaps.Profile) *Renderer
//	(*Renderer) Render(view View, prev Frame) (Frame, []byte)
//
// View bundles grid, cursor, modes, and pane bounds. Frame is the
// last-rendered state used for diffing.
//
// # Frame cache shape: grid + emitted bytes
//
// A Frame stores both:
//
//   - The grid that produced the frame (vt.Grid snapshot).
//   - The bytes that were emitted to the client to realize that grid
//     (pre-computed SGR runs, OSC 8 wrappers, kitty sequences).
//
// Storing both is redundant — bytes can be recomputed from grid.
// The redundancy is deliberate: Render's diff path needs to ask
// "which bytes correspond to the previous cells at (x, y)?" for
// every cell that changed. Recomputing byte offsets on every diff
// would quadratic over the grid. Storing the byte offsets alongside
// the grid makes lookup O(1).
//
// Memory cost: a 200x50 frame with styles is ~40KB for the grid plus
// ~30KB for the byte-map, per pane per client. Ten panes times four
// clients is under 3MB. We accept the cost — performance dominates
// memory for interactive rendering.
//
// One-time allocation per Frame; reused across renders for the same
// pane/client pair via a pool. Resize-or-window-switch triggers a
// fresh Frame (see fingerprint below) and returns the old to the pool.
//
// # Viewport fingerprint
//
// A Frame includes the vt.Viewport it was rendered from (TopLine,
// Rows, Cols). When the pane's viewport changes — scrolling through
// scrollback in copy mode, for instance — the diff renderer detects
// the mismatch and emits a full repaint for that frame instead of a
// delta. M1's naive full-repaint renderer doesn't care; M3's diff
// renderer gates on this fingerprint.
//
// The fingerprint also guards against silent desync after a Resize:
// post-resize the previous Frame's viewport and grid dimensions no
// longer match, and Render falls back to full repaint.
//
// # Corresponding tmux code
//
// tmux's tty.c + screen-redraw.c + tty-draw.c combined, narrower
// because of the closed target list and absence of terminfo.
package termout
