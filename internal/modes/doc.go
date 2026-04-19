// Package modes defines the two mode interfaces used throughout dmux
// and houses the individual mode implementations in sub-packages.
//
// # Two mode kinds
//
// PaneMode fills a single pane's rectangle and takes over that pane's
// key handling. The pane's underlying shell keeps running in the
// background — output continues to be parsed into the libghostty
// Terminal, it just isn't being displayed. Examples: copy-mode, the
// choose-tree chooser invoked inside a pane, clock-mode.
//
//	type PaneMode interface {
//	    Render(rs *RenderState, size Size)
//	    Key(k keys.Key) Outcome
//	    Mouse(ev MouseEvent) Outcome
//	    Close()
//	}
//
// ClientOverlay is drawn by render.Compose on top of the composed
// window frame, in client (screen) coordinates. It may or may not
// capture focus. Examples: menus, popup terminals, display-panes
// numerals, confirm-before dialogs, command-prompt.
//
//	type ClientOverlay interface {
//	    Rect() Rect
//	    Render(dst []Cell)
//	    Key(k keys.Key) Outcome    // called only if CaptureFocus is true
//	    Mouse(ev MouseEvent) Outcome
//	    CaptureFocus() bool
//	    Close()
//	}
//
// Outcome is one of: Consumed (stop), Passthrough (let the next layer
// handle the event), CloseMode (remove the mode/overlay), or
// Command(cmd) (enqueue a command and optionally close).
//
// # Sub-packages
//
//   - modes/copy          PaneMode, vi/emacs-style scrollback editor
//   - modes/tree          PaneMode, session/window/pane chooser
//   - modes/popup         ClientOverlay that wraps a pane.Pane running
//                         in a popup-sized PTY
//   - modes/menu          ClientOverlay, tmux-style popup menu
//   - modes/displaypanes  ClientOverlay, big numerals over each pane
//   - modes/prompt        ClientOverlay, single-line command prompt
//                         and confirm-before dialog
//
// Sub-packages are independent of each other; they all depend only on
// this package for the interfaces. modes/popup wraps a pane via the
// pane.Pane interface (passed in at construction), not the concrete
// type, so popup is testable without spawning a real PTY.
//
// # I/O surfaces
//
// None in this package. Sub-packages perform no I/O of their own —
// any side effects flow through the Outcome value (Command enqueues a
// command; CloseMode tells the caller to remove the mode) or through
// caller-supplied helpers passed at construction time.
//
// # Non-goals
//
// Modes do not reach into session state directly. They either return
// a Command outcome (enqueued by the server loop) or manipulate state
// passed in at construction time. This keeps them testable with a
// stub Server.
package modes
