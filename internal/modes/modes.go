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

// SGR attribute flags for Cell.Attrs.
const (
	AttrBold      uint8 = 1 << 0
	AttrReverse   uint8 = 1 << 1
	AttrUnderline uint8 = 1 << 2
	AttrBlink     uint8 = 1 << 3
	AttrDim       uint8 = 1 << 4
)

// Color is an 8-bit terminal color index (0–255) or one of the sentinel
// values ColorDefault (terminal default) and ColorRGB (use R,G,B fields).
type Color uint16

const (
	ColorDefault Color = 0      // terminal's default color
	ColorIndexed  Color = 0x100 // sentinel: use low byte as 256-color index
	ColorRGB      Color = 0x200 // sentinel: use R,G,B fields
)

// Cell is a single terminal display cell with styling.
type Cell struct {
	Char  rune  // displayed character; 0 is treated as a space
	Fg    Color // foreground color; ColorDefault means inherit
	Bg    Color // background color; ColorDefault means inherit
	Attrs uint8 // bitmask of Attr* constants
	// FgR, FgG, FgB are meaningful only when Fg == ColorRGB.
	FgR, FgG, FgB uint8
	// BgR, BgG, BgB are meaningful only when Bg == ColorRGB.
	BgR, BgG, BgB uint8
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
