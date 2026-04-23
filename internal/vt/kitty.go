package vt

import (
	"bytes"
	"encoding/base64"
	"strconv"
	"strings"
)

// Why this lives outside libghostty-vt:
//
// libghostty-vt's wasm32-freestanding build hard-disables kitty
// graphics (build_options.zig pins kitty_graphics=false on the wasm
// target because std.time.Instant.now is unavailable). The C ABI
// exports for placement iteration exist but every entry returns
// no_value, so we cannot recover image bytes through them.
//
// dmux's role for kitty graphics is passthrough, not rendering: when
// a child app emits APC G, kitty-capable clients want the same APC G
// bytes; everyone else wants them dropped. That is a string-level
// transformation, well below the rendering layer that libghostty-vt
// would have given us. The Go-side parser here captures the wire
// representation directly out of the pty stream, sidestepping the
// wasm gap entirely.

// Placement records one kitty graphics image transmission combined
// with its display intent. M1 only handles single-shot
// transmit+display (a=T) commands; place-existing-image (a=p),
// delete (a=d), and animation frames are tracked at the parser level
// but not surfaced.
//
// Data carries the *decoded* payload bytes (after base64 + chunk
// reassembly). For raw formats (RGB / RGBA / GRAY / GRAY_ALPHA) this
// is the pixel buffer; for PNG (Format == KittyFormatPNG) it is the
// PNG file.
//
// PixelWidth / PixelHeight come from the s= and v= keys, not from
// decoding the data. Cols / Rows are the c= / r= overrides; both
// stay zero when the sender did not specify them and let the
// receiver compute cell extents from pixel size.
type Placement struct {
	ImageID     uint32
	PlacementID uint32
	Format      KittyImageFormat
	PixelWidth  uint32
	PixelHeight uint32
	Cols, Rows  int
	Data        []byte
}

// KittyImageFormat is the f= value from the kitty graphics command.
// The values are the wire-level codes, *not* the libghostty-vt enum
// values — we never cross that boundary.
type KittyImageFormat uint8

const (
	// KittyFormatRGB is f=24 in the kitty wire protocol: 3 bytes per
	// pixel.
	KittyFormatRGB KittyImageFormat = 24
	// KittyFormatRGBA is f=32: 4 bytes per pixel.
	KittyFormatRGBA KittyImageFormat = 32
	// KittyFormatPNG is f=100: an entire PNG file.
	KittyFormatPNG KittyImageFormat = 100
)

// kittyAction is the a= value, mapped to a small enum so the parser
// does not carry a string into placement bookkeeping.
type kittyAction uint8

const (
	kittyActionUnknown        kittyAction = 0
	kittyActionTransmit       kittyAction = 't'
	kittyActionTransmitAndPut kittyAction = 'T'
	kittyActionPut            kittyAction = 'p'
	kittyActionDelete         kittyAction = 'd'
	kittyActionAnimation      kittyAction = 'a'
	kittyActionFrame          kittyAction = 'f'
	kittyActionQuery          kittyAction = 'q'
)

// kittyImageBuf accumulates chunked transmission payload (m=1 across
// multiple commands) until the terminator (m=0 or m omitted). Once
// complete it can be flushed into a Placement.
type kittyImageBuf struct {
	imageID     uint32
	placementID uint32
	format      KittyImageFormat
	pixelWidth  uint32
	pixelHeight uint32
	cols, rows  int
	display     bool
	data        bytes.Buffer
}

// kittyParser scans bytes for APC G sequences, captures the image
// payload, and exposes completed placements via take. Bytes that are
// not part of an APC G command pass through untouched via the writer
// callback the caller passes to process. The parser is stateful
// across calls so split reads (mid-APC, mid-chunked-transmission)
// reassemble correctly.
type kittyParser struct {
	state kittyState

	// Buffer for the current APC payload (between ESC _ G and ESC \).
	// We accumulate because the chunked-transmission boundary is
	// command-level, but the apc terminator is byte-level, and we
	// want to parse a whole command at once.
	apc bytes.Buffer

	// In-flight transmission keyed by image ID. The kitty protocol
	// only allows one outstanding chunked transmission at a time per
	// terminal; using a map is just defensive in case two image IDs
	// interleave (which most terminals reject, but we do not need to
	// be that strict).
	inFlight map[uint32]*kittyImageBuf

	// Completed placements waiting for the consumer. take returns and
	// clears.
	out []Placement
}

// kittyState is the streaming-parser state machine. We only need
// three states: scanning for an APC introducer, awaiting the 'G'
// that distinguishes kitty graphics from other APC payloads, and
// accumulating the APC body until the ST terminator.
type kittyState uint8

const (
	stateGround   kittyState = 0 // scanning ground bytes; ESC arms us.
	stateESC      kittyState = 1 // saw ESC; need '_' to enter APC.
	stateAPCIntro kittyState = 2 // saw ESC _; need 'G' to confirm kitty.
	stateAPCBody  kittyState = 3 // inside APC G ...; collecting until ST.
	stateAPCESC   kittyState = 4 // saw ESC inside APC body; need '\' for ST.
)

func newKittyParser() *kittyParser {
	return &kittyParser{
		inFlight: make(map[uint32]*kittyImageBuf),
	}
}

// process scans b, calls write for every contiguous run of non-APC-G
// bytes, and consumes APC G commands into the parser's in-flight
// table. Bytes inside an APC G that turns out *not* to be a kitty
// graphics command (e.g. APC for some other protocol) are flushed
// back to write so libghostty-vt's own dispatch keeps working.
//
// Returns whatever error the caller's write returned, immediately.
func (k *kittyParser) process(b []byte, write func([]byte) error) error {
	// Accumulator for ground bytes — we batch them so write isn't
	// called once per byte. flushed when state changes or on exit.
	start := 0

	flush := func(end int) error {
		if end > start {
			if err := write(b[start:end]); err != nil {
				return err
			}
		}
		start = end + 1
		return nil
	}

	for i := 0; i < len(b); i++ {
		c := b[i]
		switch k.state {
		case stateGround:
			if c == 0x1b {
				k.state = stateESC
			}

		case stateESC:
			if c == '_' {
				// Tentatively in APC: drop these two bytes from the
				// outbound stream by flushing up to *before* the ESC.
				if err := flush(i - 1); err != nil {
					return err
				}
				start = i + 1 // bytes after the '_'
				k.state = stateAPCIntro
			} else if c == 0x1b {
				// Another ESC; stay armed.
			} else {
				k.state = stateGround
			}

		case stateAPCIntro:
			if c == 'G' {
				// Confirmed kitty graphics. Drop the 'G' too.
				start = i + 1
				k.state = stateAPCBody
				k.apc.Reset()
			} else {
				// Not kitty: re-emit ESC _ and whatever this byte is,
				// then go back to ground. Re-emission lets non-kitty
				// APC payloads (like tmux, Alacritty image protocols
				// we do not capture) reach libghostty-vt.
				if err := write([]byte{0x1b, '_', c}); err != nil {
					return err
				}
				start = i + 1
				k.state = stateGround
			}

		case stateAPCBody:
			if c == 0x1b {
				k.state = stateAPCESC
			} else if c == 0x07 { // BEL terminator (legacy)
				k.commitAPC()
				start = i + 1
				k.state = stateGround
			} else {
				k.apc.WriteByte(c)
			}

		case stateAPCESC:
			if c == '\\' {
				k.commitAPC()
				start = i + 1
				k.state = stateGround
			} else {
				// Spurious ESC inside APC body: keep it as-is and
				// re-process the new byte from APC body state.
				k.apc.WriteByte(0x1b)
				k.apc.WriteByte(c)
				k.state = stateAPCBody
			}
		}
	}

	// Flush any trailing ground bytes; if we're mid-APC we already
	// advanced start past the bytes that opened it.
	if k.state == stateGround && start < len(b) {
		if err := write(b[start:]); err != nil {
			return err
		}
	}
	return nil
}

// commitAPC parses a fully accumulated APC G payload (control keys
// followed by an optional ';' and base64 payload) and updates the
// in-flight transmission table or finalises a placement.
func (k *kittyParser) commitAPC() {
	body := k.apc.Bytes()
	semi := bytes.IndexByte(body, ';')
	var ctrl, payload []byte
	if semi < 0 {
		ctrl = body
	} else {
		ctrl = body[:semi]
		payload = body[semi+1:]
	}

	keys := parseKittyKeys(ctrl)
	action := kittyActionTransmitAndPut // protocol default per kitty docs
	if v, ok := keys["a"]; ok && len(v) == 1 {
		action = kittyAction(v[0])
	}

	switch action {
	case kittyActionTransmit, kittyActionTransmitAndPut:
		k.handleTransmit(keys, payload, action == kittyActionTransmitAndPut)
	case kittyActionPut:
		k.handlePut(keys)
	default:
		// delete / animation / frame / query: not surfaced in M1.
		// We still consume the bytes so they do not leak to the vt.
	}
}

// handleTransmit accepts a chunk of an image transmission. When more
// chunks are coming (m=1) the data accumulates in inFlight. When
// this is the last chunk (m=0 or m absent) the buffer is finalised
// and, if display is true, queued as a Placement.
func (k *kittyParser) handleTransmit(keys map[string]string, payload []byte, display bool) {
	id := keysU32(keys, "i", 0)
	more := keysU32(keys, "m", 0)

	buf, exists := k.inFlight[id]
	if !exists {
		buf = &kittyImageBuf{
			imageID:     id,
			placementID: keysU32(keys, "p", 0),
			format:      KittyImageFormat(keysU32(keys, "f", uint32(KittyFormatRGBA))),
			pixelWidth:  keysU32(keys, "s", 0),
			pixelHeight: keysU32(keys, "v", 0),
			cols:        int(keysU32(keys, "c", 0)),
			rows:        int(keysU32(keys, "r", 0)),
			display:     display,
		}
		k.inFlight[id] = buf
	} else {
		// Subsequent chunks may set display via a=T on any chunk.
		if display {
			buf.display = true
		}
	}

	// Decode this chunk. Kitty mandates standard base64; ignore
	// errors silently — a corrupted chunk drops the image.
	decoded, err := base64.StdEncoding.DecodeString(string(payload))
	if err != nil {
		delete(k.inFlight, id)
		return
	}
	buf.data.Write(decoded)

	if more == 0 {
		k.finalize(buf)
		delete(k.inFlight, id)
	}
}

// handlePut emits a placement that references an already-transmitted
// image. M1 only tracks transmissions we observed locally; if the
// referenced image is unknown the put is dropped. The kitty protocol
// allows display-only references to images placed by earlier
// connections, but dmux is single-attach for M1 so the case does
// not arise.
func (k *kittyParser) handlePut(keys map[string]string) {
	// No state retained for already-finalised images in M1 — the
	// placement was emitted at transmit time. Reaching here usually
	// means a sophisticated app re-placing an existing image, which
	// we simply ignore for now.
	_ = keys
}

// finalize copies the assembled image buffer into a Placement (when
// display is set) and resets the buffer.
func (k *kittyParser) finalize(buf *kittyImageBuf) {
	if !buf.display {
		return
	}
	p := Placement{
		ImageID:     buf.imageID,
		PlacementID: buf.placementID,
		Format:      buf.format,
		PixelWidth:  buf.pixelWidth,
		PixelHeight: buf.pixelHeight,
		Cols:        buf.cols,
		Rows:        buf.rows,
	}
	if buf.data.Len() > 0 {
		p.Data = make([]byte, buf.data.Len())
		copy(p.Data, buf.data.Bytes())
	}
	k.out = append(k.out, p)
}

// take returns and clears the buffered placements. Callers normally
// invoke this once per render frame; placements not consumed before
// the next frame's transmission stack up.
func (k *kittyParser) take() []Placement {
	if len(k.out) == 0 {
		return nil
	}
	out := k.out
	k.out = nil
	return out
}

// parseKittyKeys splits a comma-delimited key=value list into a map.
// Both sides are kept as strings; numeric coercion happens at the
// call site via keysU32 because some keys (a, t, m) are
// single-character flags.
func parseKittyKeys(b []byte) map[string]string {
	out := make(map[string]string, 8)
	if len(b) == 0 {
		return out
	}
	for _, part := range strings.Split(string(b), ",") {
		eq := strings.IndexByte(part, '=')
		if eq <= 0 {
			continue
		}
		out[part[:eq]] = part[eq+1:]
	}
	return out
}

func keysU32(keys map[string]string, k string, def uint32) uint32 {
	v, ok := keys[k]
	if !ok {
		return def
	}
	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return def
	}
	return uint32(n)
}
