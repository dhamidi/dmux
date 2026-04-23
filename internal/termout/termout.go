package termout

import (
	"bytes"
	"fmt"

	"github.com/dhamidi/dmux/internal/termcaps"
	"github.com/dhamidi/dmux/internal/vt"
)

// Current scope (M1 walking skeleton):
//
//   - Full-frame repaint on every Render: no diff, no style tracking,
//     no color, no hyperlinks, no graphics. The frame cache described
//     in doc.go (grid + emitted bytes, viewport fingerprint) is not
//     yet built — Render ignores `prev` entirely.
//   - The Profile is accepted and stored but has no effect on output.
//     All three target profiles in the real matrix (Ghostty, XTermJS,
//     WindowsTerminal) receive the same bytes; Unknown does too.
//   - Cell attributes (fg/bg color, bold, italic, SGR state) are
//     dropped. Wide cells render their leading cell only (spacer-tail
//     is skipped); spacer-head becomes a space.
//
// TODO(m1:termout-diff): store previous Grid + byte offsets on Frame;
// on Render, walk changed rows and emit minimal update instead of
// the full-frame repaint.
// TODO(m1:termout-style): track SGR runs per cell; emit color + bold +
// italic sequences as attributes change.
// TODO(m1:termout-graphics): kitty passthrough, sixel passthrough, OSC
// 8 hyperlink wrappers (deferred; see doc.go).

// View bundles the per-render inputs. Modes and Bounds are reserved
// for the composition step; M1 renders the whole grid starting at the
// tty's top-left.
type View struct {
	Grid   vt.Grid
	Cursor vt.Cursor
	// TODO(m1:termout-modes): vt.Modes once the vt package exposes it.
	// TODO(m1:termout-bounds): pane bounds for multi-pane composition.
}

// Frame is the post-render cache entry. M1 stores only the Grid and
// Cursor because the diff path is not yet implemented; the byte-map
// documented in doc.go lands with the diff renderer.
type Frame struct {
	Grid   vt.Grid
	Cursor vt.Cursor
}

// Renderer turns a View into bytes destined for one client's real
// terminal. One Renderer per (profile, client) pair — reuse across
// Render calls so future caches survive.
type Renderer struct {
	profile termcaps.Profile
}

// NewRenderer constructs a Renderer for the given profile. The
// profile is currently unused (see top-of-file scope); it is captured
// so that style/graphics branches can land without changing the
// constructor signature.
func NewRenderer(p termcaps.Profile) *Renderer {
	return &Renderer{profile: p}
}

// Render paints the view as a full-frame repaint and returns both the
// updated Frame cache and the bytes to place in a proto.Output frame.
//
// The bytes assume the client's tty is in raw mode (cmd/dmux:attach
// handles that) and has a scroll region matching the grid's row count.
// The caller is responsible for sending these bytes in order; Render
// is stateless across calls besides the Renderer's profile.
//
// Output shape:
//
//	ESC [ ? 25 l          hide cursor while painting
//	ESC [ H               home
//	<row-0 cells> ESC [ K CR LF
//	...
//	<row-(n-1) cells> ESC [ K
//	ESC [ <y+1> ; <x+1> H cursor position (1-based)
//	ESC [ ? 25 h          show cursor (if Cursor.Visible)
//
// The trailing \r\n is intentionally omitted after the last row so
// the paint never scrolls the client's tty.
func (r *Renderer) Render(view View, prev Frame) (Frame, []byte) {
	_ = prev // TODO(m1:termout-diff)

	var buf bytes.Buffer
	g := view.Grid

	buf.WriteString("\x1b[?25l")
	buf.WriteString("\x1b[H")

	for y := 0; y < g.Rows; y++ {
		row := g.Cells[y]
		for x := 0; x < g.Cols; x++ {
			c := row[x]
			if c.Wide == vt.CellSpacerTail {
				continue
			}
			ch := c.Rune
			if ch == 0 {
				ch = ' '
			}
			buf.WriteRune(ch)
		}
		buf.WriteString("\x1b[K")
		if y < g.Rows-1 {
			buf.WriteString("\r\n")
		}
	}

	// Cursor positioning is 1-based in CSI H; vt.Cursor is 0-based.
	cur := view.Cursor
	fmt.Fprintf(&buf, "\x1b[%d;%dH", cur.Y+1, cur.X+1)
	if cur.Visible {
		buf.WriteString("\x1b[?25h")
	}

	return Frame{Grid: g, Cursor: cur}, buf.Bytes()
}
