package modes

import (
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/layout"
)

// Rect describes the position and size of a region in screen coordinates.
// It is a type alias for [layout.Rect] so callers need not import layout.
//
//	Rect{X, Y, Width, Height int}
type Rect = layout.Rect

// Cell is a single terminal display cell.
type Cell struct {
	Char rune // displayed character; 0 is treated as a space
}

// Size represents the dimensions of a rectangular area in character cells.
type Size struct {
	Rows int
	Cols int
}

// Canvas is the drawing surface passed to [PaneMode.Render].
// Implementations are provided by the host (server or test stub).
type Canvas interface {
	// Size returns the drawable dimensions of the canvas.
	Size() Size
	// Set writes cell c at position (col, row), where (0,0) is the
	// top-left corner of the canvas. Out-of-bounds coordinates are
	// silently ignored.
	Set(col, row int, c Cell)
}

// OutcomeKind identifies what should happen after a mode handles an event.
type OutcomeKind int

const (
	// KindConsumed stops event propagation; no further action is taken.
	KindConsumed OutcomeKind = iota
	// KindPassthrough lets the next layer handle the event.
	KindPassthrough
	// KindCloseMode removes the mode or overlay from the active stack.
	KindCloseMode
	// KindCommand enqueues a command for the server loop; [Outcome.Cmd]
	// holds the opaque command value.
	KindCommand
)

// Outcome is returned by [PaneMode] and [ClientOverlay] event handlers.
// Use the constructor functions [Consumed], [Passthrough], [CloseMode],
// and [Command] rather than constructing Outcome literals directly.
type Outcome struct {
	Kind OutcomeKind
	// Cmd holds the command to enqueue when Kind == KindCommand.
	// It is nil for all other kinds.
	Cmd any
}

// Consumed returns an Outcome that stops event propagation.
func Consumed() Outcome { return Outcome{Kind: KindConsumed} }

// Passthrough returns an Outcome that lets the next layer handle the event.
func Passthrough() Outcome { return Outcome{Kind: KindPassthrough} }

// CloseMode returns an Outcome that removes the mode or overlay.
func CloseMode() Outcome { return Outcome{Kind: KindCloseMode} }

// Command returns an Outcome that enqueues cmd in the server loop.
func Command(cmd any) Outcome { return Outcome{Kind: KindCommand, Cmd: cmd} }

// PaneMode fills a single pane's rectangle and takes over that pane's
// key handling while the underlying shell continues running in the
// background. Examples: copy-mode, choose-tree, clock-mode.
//
// The host calls Render once per display refresh and Key/Mouse for
// every relevant input event. Close is called exactly once when the
// mode is removed from the active stack.
type PaneMode interface {
	// Render draws the mode's content onto dst. The canvas size
	// matches the pane's current rectangle.
	Render(dst Canvas)
	// Key handles a keyboard event and returns what should happen next.
	Key(k keys.Key) Outcome
	// Mouse handles a mouse event and returns what should happen next.
	Mouse(ev keys.MouseEvent) Outcome
	// Close releases any resources held by the mode.
	Close()
}

// ClientOverlay is drawn over the composed window frame in client
// (screen) coordinates. It may or may not capture keyboard focus.
// Examples: menus, popup terminals, display-panes numerals,
// confirm-before dialogs, command-prompt.
//
// The host calls Render once per display refresh and routes Key/Mouse
// events according to CaptureFocus. Close is called exactly once when
// the overlay is removed.
type ClientOverlay interface {
	// Rect returns the overlay's bounding rectangle in screen
	// (client) coordinates.
	Rect() Rect
	// Render fills dst with the overlay's cells in row-major order.
	// len(dst) == Rect().Width * Rect().Height is guaranteed by the host.
	Render(dst []Cell)
	// Key handles a keyboard event; called only when CaptureFocus
	// returns true.
	Key(k keys.Key) Outcome
	// Mouse handles a mouse event and returns what should happen next.
	Mouse(ev keys.MouseEvent) Outcome
	// CaptureFocus reports whether this overlay should receive keyboard
	// events instead of the focused pane.
	CaptureFocus() bool
	// Close releases any resources held by the overlay.
	Close()
}
