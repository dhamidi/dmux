// Package displaypanes implements `display-panes` (default: prefix q)
// — a transient overlay that draws a big numeral on each pane and
// waits for a digit key to select one.
//
// # Boundary
//
// Implements [modes.ClientOverlay]. Constructed with:
//
//   - A [modes.Rect] describing the full coverage area (typically the
//     entire window/screen).
//   - A []PaneInfo slice that lists each visible pane's ID, Number
//     (0–9), and Bounds in screen coordinates. No concrete session or
//     render types are imported.
//   - A scheduleTimeout callback: called once at construction with a
//     dismiss function. The host invokes that function after
//     display-panes-time milliseconds, which marks the overlay as
//     dismissed. The host then removes it from the client overlay
//     stack (detected via [Mode.Dismissed]).
//
// On a digit keystroke that matches a pane's Number, [Mode.Key]
// returns an [modes.Outcome] carrying [SelectPaneCommand]{PaneNumber}
// for the server to enqueue. On Escape, or a digit with no matching
// pane, it returns [modes.CloseMode].
//
// # Input types
//
// [PaneInfo] is the only input type required beyond the bounds rect:
//
//	type PaneInfo struct {
//	    ID     string     // opaque pane identifier
//	    Number int        // digit label (0–9) drawn over the pane
//	    Bounds modes.Rect // position in client (screen) coordinates
//	}
//
// # Rendering
//
// Big-numeral glyphs (3 columns × 5 rows) are drawn using a full-block
// character (█) centered within each pane's Bounds rectangle.
// Only filled cells are written; background cells are left as-is in dst.
//
// # Timeout
//
// The mode itself does not import time or any server-specific timer.
// The caller supplies a scheduleTimeout function so the host loop owns
// all real-time management:
//
//	var timer *time.Timer
//	overlay := displaypanes.New(bounds, panes, func(dismiss func()) {
//	    timer = time.AfterFunc(timeout, dismiss)
//	})
//	// ... push overlay onto client overlay stack ...
//	// When overlay.Dismissed() is true, pop it from the stack.
//
// # In isolation
//
// Testable by constructing with a stub pane list and capturing the
// dismiss callback manually — no real server, session, or timer needed.
//
// # Non-goals
//
// Does not maintain the timer itself — testability requires keeping
// time out of this package entirely.
package displaypanes
