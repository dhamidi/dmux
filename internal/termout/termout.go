package termout

import (
	"bytes"

	"github.com/dhamidi/dmux/internal/termcaps"
	"github.com/dhamidi/dmux/internal/vt"
)

// Current scope (M1 walking skeleton):
//
//   - Full-frame repaint on every Render via libghostty-vt's Formatter.
//     SGR colors, OSC 8 hyperlinks, and cursor position are emitted by
//     the formatter itself; this package only prepends the
//     hide-cursor + home + erase-display preamble and appends a
//     show-cursor postlude based on the live cursor visibility.
//   - No diff rendering, no graphics routing, no per-pane compositing.
//
// TODO(m1:termout-diff): store previous bytes + fingerprint; on Render,
// skip WriteFrame when the formatter output is byte-identical.
// TODO(m1:termout-graphics): kitty passthrough, sixel passthrough.
// TODO(m1:termout-compose): rewrite formatter output to place a pane
// inside a sub-rectangle of the real tty for multi-pane layouts.

// Renderer wraps libghostty-vt formatter output for one client. It
// carries the client's profile so the appropriate format options
// (hyperlinks only where the client supports them) are derived
// automatically. One Renderer per client; cheap to construct.
type Renderer struct {
	profile termcaps.Profile
	opts    vt.FormatOptions
}

// NewRenderer constructs a Renderer for the given profile. The profile
// decides which format options are safe (OSC 8 only when
// Features().OSC8 is set, etc.); SGR and Cursor are on for every
// target because every profile in the matrix handles 24-bit color and
// CUP at least at the 256-color level.
func NewRenderer(p termcaps.Profile) *Renderer {
	f := p.Features()
	return &Renderer{
		profile: p,
		opts: vt.FormatOptions{
			SGR:       true,
			Cursor:    true,
			Hyperlink: f.OSC8,
		},
	}
}

// FormatOptions returns the options the Renderer wants the formatter
// to apply. The server pump passes these to pane.Format before calling
// Wrap on the result.
func (r *Renderer) FormatOptions() vt.FormatOptions {
	return r.opts
}

// Wrap bookends the raw formatter output with the sequences every
// dmux client's tty needs:
//
//   - CSI ?25 l: hide cursor during the paint to avoid flicker.
//   - CSI H:     home the cursor so the formatter's row-by-row output
//                lands at row 1, column 1.
//   - CSI J:     erase to end of screen so trailing blank rows clear
//                any stale content from the previous frame (the
//                formatter trims trailing empty rows from its output).
//   - CSI ?25 h: show cursor when the pane's cursor is visible. The
//                formatter's own cursor-position sequence (CUP) is
//                already inside `formatted`; we only toggle visibility.
//
// No \r\n is appended: the formatter's tail is either a CUP or the
// last line's content, and either is a safe terminator that does not
// scroll the real tty.
func (r *Renderer) Wrap(formatted []byte, visible bool) []byte {
	var buf bytes.Buffer
	buf.Grow(len(formatted) + 16)
	buf.WriteString("\x1b[?25l\x1b[H\x1b[J")
	buf.Write(formatted)
	if visible {
		buf.WriteString("\x1b[?25h")
	}
	return buf.Bytes()
}
