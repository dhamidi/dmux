package termout

import (
	"bytes"
	"encoding/base64"
	"strconv"

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
//   - Status overlay: Wrap accepts a pre-rendered status cell-row and
//     a target totalRows, paints it at the bottom of the client tty
//     after the formatter output, and re-CUPs the cursor back onto
//     the pane's cursor position so the cursor-visibility postlude
//     lands at the right spot.
//   - No diff rendering, no graphics routing, no per-pane compositing.
//
// TODO(m1:termout-diff): store previous bytes + fingerprint; on Render,
// skip WriteFrame when the formatter output is byte-identical.
// TODO(m1:termout-graphics): kitty passthrough, sixel passthrough.
// TODO(m1:termout-compose): rewrite formatter output to place a pane
// inside a sub-rectangle of the real tty for multi-pane layouts.

// kittyChunkSize is the maximum payload length per kitty graphics
// command (the protocol caps it at 4096 base64 characters per chunk).
// Sticking to the limit means even legacy terminals that buffer
// per-APC parse it correctly.
const kittyChunkSize = 4096

// Renderer wraps libghostty-vt formatter output for one client. It
// carries the client's profile so the appropriate format options
// (hyperlinks only where the client supports them) are derived
// automatically. One Renderer per client; cheap to construct.
//
// Renderer also tracks which kitty graphics image IDs we have
// already transmitted to this client. The first frame for an image
// emits transmit+place (a=T); subsequent frames re-place the same
// image ID without resending the bytes (a=p). Multi-pane M2 will
// remap server-side image IDs into a per-client ID space here so two
// panes can independently use ID 1 without collision —
// see TODO(m1:termout-kitty-rewrite).
type Renderer struct {
	profile  termcaps.Profile
	opts     vt.FormatOptions
	sentIDs  map[uint32]struct{}
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
		sentIDs: make(map[uint32]struct{}),
	}
}

// FormatOptions returns the options the Renderer wants the formatter
// to apply. The server pump passes these to pane.Format before calling
// Wrap on the result.
func (r *Renderer) FormatOptions() vt.FormatOptions {
	return r.opts
}

// EmitKitty serialises kitty graphics placements as APC G commands
// for capable clients. Returns nil bytes when the renderer's profile
// does not support kitty graphics (see termcaps.Features.KittyGraphics)
// or when placements is empty.
//
// The first time a given ImageID is seen, EmitKitty emits a full
// transmit+display command (a=T) carrying the base64-encoded image
// data, chunked at the protocol's per-command limit. Subsequent
// frames for the same ImageID emit a place-only command (a=p) so we
// do not retransmit static images on every render.
//
// Output ordering: the formatter wrap (Wrap) emits cursor home +
// erase, then the cell grid, then the cursor restore. APC G commands
// emitted here go *after* the wrap so the image overlays the
// formatter's grid at whatever cursor position the formatter left.
// The kitty terminal is responsible for placing the image at the
// current cursor unless the command specifies otherwise.
//
// TODO(m1:termout-kitty-rewrite): when multiple panes share one
// client, image IDs from different panes must be rewritten into a
// per-client ID space to avoid collision. The hook is here: walk
// placements once, allocate a fresh client-side ID per
// (paneID, ImageID), substitute it into the emitted commands. M1 is
// single-pane so the substitution is the identity.
func (r *Renderer) EmitKitty(placements []vt.Placement) []byte {
	if len(placements) == 0 {
		return nil
	}
	if !r.profile.Features().KittyGraphics {
		return nil
	}

	var buf bytes.Buffer
	for _, p := range placements {
		_, alreadySent := r.sentIDs[p.ImageID]
		if alreadySent {
			r.emitPlace(&buf, p)
			continue
		}
		r.emitTransmitAndPlace(&buf, p)
		r.sentIDs[p.ImageID] = struct{}{}
	}
	return buf.Bytes()
}

// emitTransmitAndPlace writes a chunked APC G transmission for p,
// using a=T on the first chunk and a=t on the rest. The kitty
// protocol requires a=T (or a=t followed by a=p) on the *first*
// chunk and m=1 on every chunk except the last; m=0 on the last
// chunk also signals "now display."
func (r *Renderer) emitTransmitAndPlace(buf *bytes.Buffer, p vt.Placement) {
	encoded := base64.StdEncoding.EncodeToString(p.Data)

	// Single-chunk fast path: build one APC with a=T plus the full
	// payload. Most placements (raw RGB, small PNGs) fit.
	if len(encoded) <= kittyChunkSize {
		buf.WriteString("\x1b_G")
		writeKittyHeader(buf, p, true /* withDisplay */, false /* more */)
		buf.WriteByte(';')
		buf.WriteString(encoded)
		buf.WriteString("\x1b\\")
		return
	}

	// Multi-chunk: first chunk carries the metadata + a=T + m=1,
	// middle chunks carry only m=1, last chunk carries m=0.
	for i := 0; i < len(encoded); i += kittyChunkSize {
		end := i + kittyChunkSize
		if end > len(encoded) {
			end = len(encoded)
		}
		isFirst := i == 0
		isLast := end == len(encoded)

		buf.WriteString("\x1b_G")
		if isFirst {
			writeKittyHeader(buf, p, true /* withDisplay */, !isLast)
		} else {
			// Continuation chunks only carry the more flag.
			buf.WriteString("m=")
			if isLast {
				buf.WriteByte('0')
			} else {
				buf.WriteByte('1')
			}
		}
		buf.WriteByte(';')
		buf.WriteString(encoded[i:end])
		buf.WriteString("\x1b\\")
	}
}

// emitPlace writes a place-existing-image command for p. We drop
// payload bytes entirely; the receiving terminal already has the
// image keyed by ImageID from our earlier transmit.
func (r *Renderer) emitPlace(buf *bytes.Buffer, p vt.Placement) {
	buf.WriteString("\x1b_Ga=p,i=")
	buf.WriteString(strconv.FormatUint(uint64(p.ImageID), 10))
	if p.PlacementID != 0 {
		buf.WriteString(",p=")
		buf.WriteString(strconv.FormatUint(uint64(p.PlacementID), 10))
	}
	buf.WriteString(",q=2")
	buf.WriteString("\x1b\\")
}

// writeKittyHeader writes the comma-delimited control fields for an
// APC G transmission command (everything between "\x1b_G" and ";").
// withDisplay toggles a=T vs a=t; more sets m=1 vs m=0. The image
// metadata (format, dimensions, ID) is always emitted on the first
// chunk so the receiver can size its decoder.
func writeKittyHeader(buf *bytes.Buffer, p vt.Placement, withDisplay, more bool) {
	if withDisplay {
		buf.WriteString("a=T")
	} else {
		buf.WriteString("a=t")
	}
	buf.WriteString(",f=")
	buf.WriteString(strconv.FormatUint(uint64(p.Format), 10))
	buf.WriteString(",i=")
	buf.WriteString(strconv.FormatUint(uint64(p.ImageID), 10))
	if p.PlacementID != 0 {
		buf.WriteString(",p=")
		buf.WriteString(strconv.FormatUint(uint64(p.PlacementID), 10))
	}
	if p.PixelWidth != 0 {
		buf.WriteString(",s=")
		buf.WriteString(strconv.FormatUint(uint64(p.PixelWidth), 10))
	}
	if p.PixelHeight != 0 {
		buf.WriteString(",v=")
		buf.WriteString(strconv.FormatUint(uint64(p.PixelHeight), 10))
	}
	if p.Cols != 0 {
		buf.WriteString(",c=")
		buf.WriteString(strconv.Itoa(p.Cols))
	}
	if p.Rows != 0 {
		buf.WriteString(",r=")
		buf.WriteString(strconv.Itoa(p.Rows))
	}
	buf.WriteString(",q=2")
	if more {
		buf.WriteString(",m=1")
	} else {
		buf.WriteString(",m=0")
	}
}

// Wrap bookends the raw formatter output with the sequences every
// dmux client's tty needs, and overlays a status cell-row at the
// bottom of the client's tty:
//
//   - CSI ?25 l: hide cursor during the paint to avoid flicker.
//   - CSI H:     home the cursor so the formatter's row-by-row output
//                lands at row 1, column 1.
//   - CSI J:     erase to end of screen so trailing blank rows clear
//                any stale content from the previous frame (the
//                formatter trims trailing empty rows from its output).
//   - formatted: the libghostty-vt formatter output, including the
//                pane's own CUP to the pane cursor.
//   - status:    when statusRow != nil, CUP to (totalRows, 1), write
//                statusRow verbatim, then CUP back to the pane cursor
//                at (cursor.Y+1, cursor.X+1) so the post-paint cursor
//                sits where the pane expects.
//   - CSI ?25 h: show cursor when the pane's cursor is visible. The
//                formatter's own cursor-position sequence (CUP) is
//                already inside `formatted`; we only toggle visibility.
//
// When statusRow is nil, the status overlay and the cursor-restore
// CUP are skipped; behaviour matches the original Wrap signature so
// callers that haven't wired status yet keep working.
//
// No \r\n is appended: the tail is either the cursor-restore CUP, the
// formatter's own CUP, or the last line's content; all are safe
// terminators that do not scroll the real tty.
func (r *Renderer) Wrap(formatted []byte, cursor vt.Cursor, statusRow []byte, totalRows int) []byte {
	var buf bytes.Buffer
	buf.Grow(len(formatted) + len(statusRow) + 32)
	buf.WriteString("\x1b[?25l\x1b[H\x1b[J")
	buf.Write(formatted)
	if statusRow != nil {
		buf.WriteString("\x1b[")
		buf.WriteString(strconv.Itoa(totalRows))
		buf.WriteString(";1H")
		buf.Write(statusRow)
		buf.WriteString("\x1b[")
		buf.WriteString(strconv.Itoa(cursor.Y + 1))
		buf.WriteByte(';')
		buf.WriteString(strconv.Itoa(cursor.X + 1))
		buf.WriteByte('H')
	}
	if cursor.Visible {
		buf.WriteString("\x1b[?25h")
	}
	return buf.Bytes()
}
