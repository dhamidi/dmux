// Package lock implements a full-screen lock overlay for dmux clients.
//
// While the overlay is active every key event is consumed; no input reaches
// the underlying pane or key table. Rendered output replaces the normal
// screen with a centred lock prompt. The overlay closes only when the user
// enters the correct passphrase.
package lock

import (
	"strings"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
)

// Mode is a full-screen client overlay that locks the terminal until the
// correct passphrase is entered.
//
// Construct with [New]; do not copy after first use.
type Mode struct {
	rect   modes.Rect
	buf    []rune // passphrase accumulation buffer
	errMsg string // empty unless the last attempt was wrong

	// verify is called with the entered passphrase on Enter. It returns
	// true when the passphrase is correct and the overlay should close.
	verify func(passphrase string) bool
}

// New creates a lock overlay covering rect. verify is called with the
// passphrase the user typed when they press Enter; it returns true on
// success.
func New(rect modes.Rect, verify func(passphrase string) bool) *Mode {
	return &Mode{
		rect:   rect,
		verify: verify,
	}
}

// Rect returns the overlay's bounding rectangle in screen coordinates.
func (m *Mode) Rect() modes.Rect { return m.rect }

// Render fills dst with the lock screen in row-major order.
// len(dst) == Rect().Width * Rect().Height is guaranteed by the host.
func (m *Mode) Render(dst []modes.Cell) {
	w := m.rect.Width
	h := m.rect.Height

	// Fill the entire screen with reverse-video spaces.
	for i := range dst {
		dst[i] = modes.Cell{Char: ' ', Attrs: modes.AttrReverse}
	}

	if w <= 0 || h <= 0 {
		return
	}

	// Title line: "Screen Locked"
	title := "Screen Locked"
	titleRow := h/2 - 2
	if titleRow < 0 {
		titleRow = 0
	}
	writeStr(dst, w, h, titleRow, title, modes.AttrReverse|modes.AttrBold)

	// Password line: "Password: ***_"
	stars := strings.Repeat("*", len(m.buf))
	prompt := "Password: " + stars + "_"
	promptRow := h / 2
	writeStr(dst, w, h, promptRow, prompt, modes.AttrReverse)

	// Error line (shown only after a failed attempt).
	if m.errMsg != "" {
		errRow := h/2 + 2
		if errRow >= h {
			errRow = h - 1
		}
		writeStr(dst, w, h, errRow, m.errMsg, modes.AttrReverse)
	}
}

// writeStr writes the string s centred on row in dst (row-major, width w,
// height h). Attrs is applied to every cell written.
func writeStr(dst []modes.Cell, w, h, row int, s string, attrs uint8) {
	if row < 0 || row >= h {
		return
	}
	runes := []rune(s)
	col := (w - len(runes)) / 2
	if col < 0 {
		col = 0
	}
	for _, r := range runes {
		idx := row*w + col
		if col >= w || idx >= len(dst) {
			break
		}
		dst[idx] = modes.Cell{Char: r, Attrs: attrs}
		col++
	}
}

// Key handles a keyboard event. Every key is consumed; no input reaches
// the pane below. On Enter the accumulated passphrase is verified:
//   - correct passphrase → overlay closes (CloseMode outcome).
//   - incorrect passphrase → buffer cleared, error message shown.
//
// Escape clears the buffer silently. Backspace removes the last character.
// Any other printable character is appended to the buffer.
func (m *Mode) Key(k keys.Key) modes.Outcome {
	switch k.Code {
	case keys.CodeEnter:
		passphrase := string(m.buf)
		m.buf = m.buf[:0]
		m.errMsg = ""
		if m.verify(passphrase) {
			return modes.CloseMode()
		}
		m.errMsg = "Incorrect passphrase"

	case keys.CodeBackspace:
		m.errMsg = ""
		if len(m.buf) > 0 {
			m.buf = m.buf[:len(m.buf)-1]
		}

	case keys.CodeEscape:
		// Locked – Escape does not unlock; it only clears the buffer.
		m.buf = m.buf[:0]
		m.errMsg = ""

	default:
		// Append printable characters (no modifier).
		if k.Code > 0 && k.Mod == 0 {
			m.buf = append(m.buf, rune(k.Code))
			m.errMsg = ""
		}
	}

	// Always consume – nothing passes through while locked.
	return modes.Consumed()
}

// Mouse consumes all mouse events while the screen is locked.
func (m *Mode) Mouse(_ keys.MouseEvent) modes.Outcome { return modes.Consumed() }

// CaptureFocus returns true so all keyboard events are delivered to the lock
// overlay instead of the focused pane.
func (m *Mode) CaptureFocus() bool { return true }

// Close is a no-op; the lock overlay holds no external resources.
func (m *Mode) Close() {}

// Buf returns the current passphrase buffer contents. Useful in tests.
func (m *Mode) Buf() string { return string(m.buf) }

// ErrMsg returns the current error message. Useful in tests.
func (m *Mode) ErrMsg() string { return m.errMsg }
