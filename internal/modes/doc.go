// Package modes defines the two mode interfaces used throughout dmux
// and houses the individual mode implementations in sub-packages.
//
// # Shared types
//
// [Rect] is a type alias for layout.Rect:
//
//	type Rect = layout.Rect  // {X, Y, Width, Height int}
//
// [Cell] is a single terminal display cell:
//
//	type Cell struct { Char rune }
//
// [Size] holds drawable dimensions:
//
//	type Size struct { Rows, Cols int }
//
// [Canvas] is the drawing surface passed to [PaneMode.Render]:
//
//	type Canvas interface {
//	    Size() Size
//	    Set(col, row int, c Cell)
//	}
//
// [Outcome] is returned by all Key and Mouse handlers:
//
//	type Outcome struct {
//	    Kind OutcomeKind  // KindConsumed, KindPassthrough, KindCloseMode, KindCommand
//	    Cmd  any          // non-nil when Kind == KindCommand
//	}
//
// Use the constructor functions rather than Outcome literals:
// [Consumed], [Passthrough], [CloseMode], [Command].
//
// # Two mode kinds
//
// [PaneMode] fills a single pane's rectangle and takes over that pane's
// key handling. The pane's underlying shell keeps running in the
// background — output continues to be parsed into the libghostty
// Terminal, it just isn't being displayed. Examples: copy-mode, the
// choose-tree chooser invoked inside a pane, clock-mode.
//
//	type PaneMode interface {
//	    Render(dst Canvas)
//	    Key(k keys.Key) Outcome
//	    Mouse(ev keys.MouseEvent) Outcome
//	    Close()
//	}
//
// [ClientOverlay] is drawn by render.Compose on top of the composed
// window frame, in client (screen) coordinates. It may or may not
// capture focus. Examples: menus, popup terminals, display-panes
// numerals, confirm-before dialogs, command-prompt.
//
//	type ClientOverlay interface {
//	    Rect() Rect
//	    Render(dst []Cell)
//	    Key(k keys.Key) Outcome      // called only if CaptureFocus returns true
//	    Mouse(ev keys.MouseEvent) Outcome
//	    CaptureFocus() bool
//	    Close()
//	}
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
// this package for the interfaces and shared types.
//
// # Module boundary
//
// This package imports only [github.com/dhamidi/dmux/internal/keys]
// (for keys.Key and keys.MouseEvent) and
// [github.com/dhamidi/dmux/internal/layout] (for the Rect alias).
// Both are pure leaf packages with no internal dependencies of their
// own. No concrete render, pane, or session types are imported.
//
// # Non-goals
//
// Modes do not reach into session state directly. They either return
// a Command outcome (enqueued by the server loop) or manipulate state
// passed in at construction time. This keeps them testable with a
// fake Server.
package modes
