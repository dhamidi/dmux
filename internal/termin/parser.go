package termin

import (
	"time"
	"unicode/utf8"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/termcaps"
)

// escTimeout is the grace period the parser waits after a bare ESC
// before giving up on an escape sequence and emitting Escape alone.
// Matches tmux's escape-time default. The parser does not read a
// clock; the server drives the deadline via Tick.
const escTimeout = 25 * time.Millisecond

// parserState is the high-level state of the byte-level decoder.
// The names mirror the ECMA-48 / xterm state machine but are
// collapsed because termin does not re-emit sequences — it only
// classifies them.
type parserState uint8

const (
	stateGround   parserState = iota // no in-flight sequence
	stateEsc                         // ESC received, awaiting next byte
	stateCSI                         // ESC [ received, gathering parameters/intermediates
	stateSS3                         // ESC O received, awaiting one final byte
	statePaste                       // between CSI 200 ~ and CSI 201 ~
	stateUTF8                        // inside a multi-byte UTF-8 codepoint
)

// Parser converts raw bytes from a client's terminal into Events.
// A Parser is tied to one termcaps.Profile: branching on profile
// features (notably KKP) happens at decode time, not at
// construction time, so a Parser is cheap and stateless across
// sessions.
//
// Parser is not safe for concurrent use; it is owned by a single
// per-client goroutine.
type Parser struct {
	features termcaps.Profile
	feats    termcaps.Features

	state parserState

	// csiBuf accumulates CSI parameter/intermediate bytes. We cap
	// it so a runaway terminal cannot make the parser grow
	// unbounded; real CSI sequences are short.
	csiBuf [64]byte
	csiLen int

	// utfBuf accumulates the bytes of an in-flight UTF-8 rune.
	utfBuf [4]byte
	utfLen int

	// escDeadline is the time at which a pending lone ESC should
	// be emitted as KeyEscape. Zero means "no ESC pending".
	escDeadline time.Time
	escPending  bool

	// pasteBuf accumulates data between CSI 200 ~ and CSI 201 ~.
	pasteBuf []byte

	// out is the event list under construction for the current
	// Feed call. Reused across calls; reset at the top of Feed.
	out []Event
}

// NewParser returns a Parser primed for profile p. The returned
// Parser is in ground state: no pending ESC, no paste accumulator,
// no half-decoded sequence.
func NewParser(p termcaps.Profile) *Parser {
	return &Parser{
		features: p,
		feats:    p.Features(),
	}
}

// Feed consumes b and returns the events recognized in it. The
// returned slice is only valid until the next call to Feed or
// Tick; callers must copy events they intend to hold on to.
//
// Feed never emits more than one event per input rune/sequence.
// A bare ESC byte does not produce an event; see Tick and
// Deadline for the escape-timeout protocol.
func (p *Parser) Feed(b []byte) []Event {
	p.out = p.out[:0]
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch p.state {
		case stateGround:
			p.feedGround(c)
		case stateEsc:
			p.feedEsc(c)
		case stateCSI:
			p.feedCSI(c)
		case stateSS3:
			p.feedSS3(c)
		case statePaste:
			// The end marker is CSI 201 ~. We detect the ESC that
			// starts it, then route subsequent bytes through the
			// CSI state machine until the ~ arrives. Until then,
			// raw bytes accumulate in pasteBuf.
			if c == 0x1B {
				p.state = stateEsc
				p.escPending = false // inside a paste ESC cannot stand alone
			} else {
				p.pasteBuf = append(p.pasteBuf, c)
			}
		case stateUTF8:
			p.feedUTF8(c)
		}
	}
	return p.out
}

// Tick fires pending time-driven events. The caller passes the
// current time; Parser has no clock of its own. If a bare ESC was
// buffered and now is at or past the ESC deadline, Tick emits a
// KeyEscape event and clears the pending state. Otherwise Tick
// returns an empty slice.
func (p *Parser) Tick(now time.Time) []Event {
	p.out = p.out[:0]
	if p.escPending && !now.Before(p.escDeadline) {
		p.emitKey(keys.KeyEscape, 0, "")
		p.escPending = false
		p.escDeadline = time.Time{}
		p.state = stateGround
	}
	return p.out
}

// Deadline returns the time at which Tick should next be called,
// or ok=false if the parser has no pending timer. The server uses
// this to arm a timer on the minimum of all clients' deadlines.
func (p *Parser) Deadline() (time.Time, bool) {
	if p.escPending {
		return p.escDeadline, true
	}
	return time.Time{}, false
}

// Reset drops all pending state: the ESC buffer, the paste
// accumulator, any in-flight CSI/SS3/UTF-8 sequence, and the
// escape deadline. After Reset the Parser is equivalent to a
// freshly constructed one with the same profile.
func (p *Parser) Reset() {
	p.state = stateGround
	p.csiLen = 0
	p.utfLen = 0
	p.escPending = false
	p.escDeadline = time.Time{}
	p.pasteBuf = p.pasteBuf[:0]
	p.out = p.out[:0]
}

// feedGround handles the no-in-flight-sequence case. Every byte
// either starts a new sequence (ESC, UTF-8 lead) or maps directly
// to a single-byte key event (C0 control, printable ASCII).
func (p *Parser) feedGround(c byte) {
	switch {
	case c == 0x1B:
		// ESC: start a new sequence. Do not emit yet; the next
		// byte (or a Tick past the deadline) resolves it.
		p.state = stateEsc
		p.escPending = true
		p.escDeadline = time.Now().Add(escTimeout)
	case c == 0x00:
		p.emitKey(keys.KeySpace, keys.ModCtrl, "")
	case c >= 0x01 && c <= 0x1A:
		// Ctrl-A..Ctrl-Z, except the special cases above. 0x08
		// (BS), 0x09 (HT), 0x0A (LF), 0x0D (CR) are more useful
		// as the keycap keys the user actually pressed.
		switch c {
		case 0x08:
			p.emitKey(keys.KeyBackspace, 0, "")
		case 0x09:
			p.emitKey(keys.KeyTab, 0, "")
		case 0x0A, 0x0D:
			p.emitKey(keys.KeyEnter, 0, "")
		default:
			p.emitKey(keys.KeyA+keys.Key(c-1), keys.ModCtrl, "")
		}
	case c == 0x1C:
		p.emitKey(keys.KeyBackslash, keys.ModCtrl, "")
	case c == 0x1D:
		p.emitKey(keys.KeyBracketRight, keys.ModCtrl, "")
	case c == 0x1E:
		// Ctrl-^ is Ctrl-Shift-6 on US keyboards; no dedicated
		// Key exists, so emit with Key=Digit6 and Mods=Ctrl|Shift.
		// This is the same shape tmux's tty-keys.c settles on.
		p.emitKey(keys.KeyDigit6, keys.ModCtrl|keys.ModShift, "")
	case c == 0x1F:
		// Ctrl-_ maps to Ctrl-Shift-- on US keyboards; again
		// there is no dedicated keycap.
		p.emitKey(keys.KeyMinus, keys.ModCtrl|keys.ModShift, "")
	case c == 0x7F:
		p.emitKey(keys.KeyBackspace, 0, "")
	case c >= 0x20 && c <= 0x7E:
		p.emitPrintable(c)
	case c >= 0x80:
		// UTF-8 leading byte. Fall into the UTF-8 accumulator.
		p.utfBuf[0] = c
		p.utfLen = 1
		p.state = stateUTF8
	}
}

// feedEsc handles the byte after a bare ESC. This decides between
// CSI (ESC [), SS3 (ESC O), Alt-prefix (ESC <printable>), or
// "ESC then a fresh sequence" (two escapes in a row: emit the
// first as Escape, start the second).
func (p *Parser) feedEsc(c byte) {
	p.escPending = false
	p.escDeadline = time.Time{}
	switch c {
	case '[':
		p.state = stateCSI
		p.csiLen = 0
	case 'O':
		p.state = stateSS3
	case 0x1B:
		// ESC ESC: the first was a real Escape; the second starts
		// something new.
		p.emitKey(keys.KeyEscape, 0, "")
		p.state = stateEsc
		p.escPending = true
		p.escDeadline = time.Now().Add(escTimeout)
	default:
		// ESC <anything else>: Alt-prefix. Printable ASCII and
		// controls alike become the same key they would be alone,
		// with ModAlt added.
		p.state = stateGround
		before := len(p.out)
		p.feedGround(c)
		after := len(p.out)
		// Add ModAlt to every event produced (normally one).
		for i := before; i < after; i++ {
			ke, ok := p.out[i].(KeyEvent)
			if !ok {
				continue
			}
			ke.Mods |= keys.ModAlt
			p.out[i] = ke
		}
	}
}

// feedCSI accumulates the body of a CSI sequence. A CSI ends on
// the first byte in the range 0x40..0x7E; everything before is a
// parameter byte (0x30..0x3F) or intermediate byte (0x20..0x2F).
// We do not validate strictly — the dispatch table reads whatever
// parameters we managed to collect and does best-effort matching.
func (p *Parser) feedCSI(c byte) {
	// Final byte.
	if c >= 0x40 && c <= 0x7E {
		// Default to ground; dispatch may override (e.g. paste
		// start transitions to statePaste).
		p.state = stateGround
		p.dispatchCSI(c)
		p.csiLen = 0
		return
	}
	// Body byte: accumulate if we have room, otherwise drop the
	// byte but stay in CSI — the final will still close the
	// sequence cleanly. Runaway CSI does not corrupt downstream
	// state.
	if p.csiLen < len(p.csiBuf) {
		p.csiBuf[p.csiLen] = c
		p.csiLen++
	}
}

// feedSS3 handles ESC O <final>. The final byte is one of
// A/B/C/D (arrows in application mode) or P/Q/R/S (F1..F4).
// Anything else is silently consumed.
func (p *Parser) feedSS3(c byte) {
	p.state = stateGround
	if k, ok := csiFinalKey[c]; ok {
		p.emitKey(k, 0, "")
	}
}

// feedUTF8 gathers continuation bytes of a multi-byte UTF-8
// codepoint. When the buffer holds a complete rune we emit one
// KeyEvent; malformed sequences emit an Unidentified key with the
// raw lead byte so the stream does not silently drop data.
func (p *Parser) feedUTF8(c byte) {
	if p.utfLen < len(p.utfBuf) {
		p.utfBuf[p.utfLen] = c
		p.utfLen++
	}
	if utf8.FullRune(p.utfBuf[:p.utfLen]) {
		r, size := utf8.DecodeRune(p.utfBuf[:p.utfLen])
		if r == utf8.RuneError && size <= 1 {
			// Bad lead byte: emit one Unidentified carrying the
			// raw byte and advance one position.
			p.emitBadByte(p.utfBuf[0])
			// The remaining bytes, if any, go back through the
			// ground state.
			for i := 1; i < p.utfLen; i++ {
				p.feedGround(p.utfBuf[i])
			}
		} else {
			p.out = append(p.out, KeyEvent{Event: keys.Event{
				Action:    keys.ActionPress,
				Key:       keys.KeyUnidentified,
				Text:      string(p.utfBuf[:size]),
				Unshifted: r,
			}})
			// Continuation bytes past the rune (shouldn't happen
			// with FullRune but handle defensively).
			for i := size; i < p.utfLen; i++ {
				p.feedGround(p.utfBuf[i])
			}
		}
		p.utfLen = 0
		p.state = stateGround
	}
}

// emitBadByte records a single malformed byte as an Unidentified
// key event whose Text is the raw byte. Callers log this shape
// when they diagnose input issues.
func (p *Parser) emitBadByte(c byte) {
	p.out = append(p.out, KeyEvent{Event: keys.Event{
		Action: keys.ActionPress,
		Key:    keys.KeyUnidentified,
		Text:   string([]byte{c}),
	}})
}

// emitKey appends a plain press event for (k, mods, text) to the
// current output buffer. text is empty for function keys and
// control characters; for printable keys it is the UTF-8 of the
// keycap.
func (p *Parser) emitKey(k keys.Key, mods keys.Mods, text string) {
	p.out = append(p.out, KeyEvent{Event: keys.Event{
		Action: keys.ActionPress,
		Key:    k,
		Mods:   mods,
		Text:   text,
	}})
}

// emitPrintable turns one byte in the ASCII printable range into
// a KeyEvent. The Key is looked up via the static ASCII table;
// Text carries the literal byte so downstream code can
// reconstruct exactly what the terminal reported. Uppercase
// letters carry ModShift, matching tmux/keys' shift-normalize
// contract.
func (p *Parser) emitPrintable(c byte) {
	k := asciiKey[c]
	var mods keys.Mods
	if c >= 'A' && c <= 'Z' {
		mods = keys.ModShift
	}
	p.emitKey(k, mods, string([]byte{c}))
}

// asciiKey maps the printable ASCII range to keys.Key. The table
// is indexed by byte value so c >= 0x20 && c <= 0x7E can index
// directly. Unmapped slots fall back to KeyUnidentified.
var asciiKey = func() [128]keys.Key {
	var t [128]keys.Key
	// Digits.
	for i := byte('0'); i <= '9'; i++ {
		t[i] = keys.KeyDigit0 + keys.Key(i-'0')
	}
	// Letters — both cases map to the same keys.Key (shift is in Mods).
	for i := byte('a'); i <= 'z'; i++ {
		t[i] = keys.KeyA + keys.Key(i-'a')
	}
	for i := byte('A'); i <= 'Z'; i++ {
		t[i] = keys.KeyA + keys.Key(i-'A')
	}
	// Space.
	t[' '] = keys.KeySpace
	// Punctuation.
	t['`'] = keys.KeyBackquote
	t['~'] = keys.KeyBackquote
	t['-'] = keys.KeyMinus
	t['_'] = keys.KeyMinus
	t['='] = keys.KeyEqual
	t['+'] = keys.KeyEqual
	t['['] = keys.KeyBracketLeft
	t['{'] = keys.KeyBracketLeft
	t[']'] = keys.KeyBracketRight
	t['}'] = keys.KeyBracketRight
	t['\\'] = keys.KeyBackslash
	t['|'] = keys.KeyBackslash
	t[';'] = keys.KeySemicolon
	t[':'] = keys.KeySemicolon
	t['\''] = keys.KeyQuote
	t['"'] = keys.KeyQuote
	t[','] = keys.KeyComma
	t['<'] = keys.KeyComma
	t['.'] = keys.KeyPeriod
	t['>'] = keys.KeyPeriod
	t['/'] = keys.KeySlash
	t['?'] = keys.KeySlash
	// Shifted digit row — the keycap IS the digit key.
	t['!'] = keys.KeyDigit1
	t['@'] = keys.KeyDigit2
	t['#'] = keys.KeyDigit3
	t['$'] = keys.KeyDigit4
	t['%'] = keys.KeyDigit5
	t['^'] = keys.KeyDigit6
	t['&'] = keys.KeyDigit7
	t['*'] = keys.KeyDigit8
	t['('] = keys.KeyDigit9
	t[')'] = keys.KeyDigit0
	return t
}()

// dispatchCSI is called when the CSI final byte arrives. It
// inspects p.csiBuf[:p.csiLen] together with the final byte and
// emits the matched event, or silently drops the sequence if no
// pattern matches.
func (p *Parser) dispatchCSI(final byte) {
	body := p.csiBuf[:p.csiLen]

	// Bracketed-paste start/end is shaped "CSI 200 ~" / "CSI 201 ~".
	// They are common enough that we handle them before generic
	// "CSI n ~" decoding.
	if final == '~' && len(body) == 3 {
		switch string(body) {
		case "200":
			p.state = statePaste
			p.pasteBuf = p.pasteBuf[:0]
			return
		case "201":
			// End of paste: emit accumulated bytes.
			data := make([]byte, len(p.pasteBuf))
			copy(data, p.pasteBuf)
			p.pasteBuf = p.pasteBuf[:0]
			p.out = append(p.out, PasteEvent{Data: data})
			return
		}
	}

	// Focus in / out: CSI I / CSI O with no parameters. Note that
	// SS3 O is consumed separately; by the time we land here we
	// know the sequence had a '[' prefix.
	if len(body) == 0 {
		switch final {
		case 'I':
			p.out = append(p.out, FocusEvent{In: true})
			return
		case 'O':
			p.out = append(p.out, FocusEvent{In: false})
			return
		}
	}

	// SGR mouse: CSI < Cb ; Cx ; Cy M/m. Body begins with '<'.
	// We recognize the envelope and emit a placeholder MouseEvent
	// so the bytes do not leak through as stray keys.
	if len(body) > 0 && body[0] == '<' && (final == 'M' || final == 'm') {
		p.out = append(p.out, MouseEvent{Button: MouseButtonNone, Press: final == 'M'})
		return
	}

	// KKP CSI-u: "CSI codepoint[:...][ ; modifier[:event-type] ] u".
	// Feature-gated on the profile's KKP flag because non-KKP
	// terminals also use 'u' as a CSI final for a few legacy
	// sequences that we would otherwise misinterpret.
	if final == 'u' && p.feats.KKP {
		p.dispatchKKP(body)
		return
	}

	// "CSI n ~" family: Home, Insert, Delete, End, PgUp, PgDn, F-keys.
	if final == '~' {
		params := parseParams(body)
		if len(params) == 0 {
			return
		}
		k, ok := csiTildeKey[params[0]]
		if !ok {
			return
		}
		var mods keys.Mods
		if len(params) >= 2 {
			mods = xtermModsToMods(params[1])
		}
		p.emitKey(k, mods, "")
		return
	}

	// Letter finals A..H, P..S: cursor keys and F1..F4.
	if k, ok := csiFinalKey[final]; ok {
		// With no parameters: plain arrow / F-key.
		if len(body) == 0 {
			p.emitKey(k, 0, "")
			return
		}
		// "CSI 1 ; mod <letter>" carries a modifier.
		params := parseParams(body)
		if len(params) >= 2 {
			p.emitKey(k, xtermModsToMods(params[1]), "")
			return
		}
		// Single parameter before a letter: ignore; xterm does
		// not define this shape and tmux drops it silently.
		return
	}

	// Unrecognized: silently drop. The parser must never leak
	// CSI bodies as stray key events.
}

// dispatchKKP decodes a KKP CSI-u body into a KeyEvent. Best
// effort: if the body is too weird to parse we drop it. tmux
// takes the same approach.
func (p *Parser) dispatchKKP(body []byte) {
	// Split on ';'. First sub-section is the codepoint (possibly
	// a colon-separated alternate form we ignore). Second is the
	// modifier + optional event type, separated by ':'. Third is
	// associated text — ignored for M1.
	sections := splitByte(body, ';')
	if len(sections) == 0 || len(sections[0]) == 0 {
		return
	}
	cpParts := splitByte(sections[0], ':')
	cp, ok := parseInt(cpParts[0])
	if !ok {
		return
	}

	var mods keys.Mods
	action := keys.ActionPress
	if len(sections) >= 2 {
		modParts := splitByte(sections[1], ':')
		if len(modParts) >= 1 {
			if n, ok := parseInt(modParts[0]); ok {
				mods = kkpModsToMods(n)
			}
		}
		if len(modParts) >= 2 {
			if n, ok := parseInt(modParts[1]); ok {
				action = kkpEventTypeToAction(n)
			}
		}
	}

	// Decide key + text from the codepoint.
	var k keys.Key
	var text string
	switch {
	case cp >= 'a' && cp <= 'z':
		k = keys.KeyA + keys.Key(cp-'a')
		text = string(rune(cp))
	case cp >= 'A' && cp <= 'Z':
		k = keys.KeyA + keys.Key(cp-'A')
		mods |= keys.ModShift
		text = string(rune(cp))
	case cp >= '0' && cp <= '9':
		k = keys.KeyDigit0 + keys.Key(cp-'0')
		text = string(rune(cp))
	case cp == ' ':
		k = keys.KeySpace
		text = " "
	case cp == 0x1B:
		k = keys.KeyEscape
	case cp == 0x0D:
		k = keys.KeyEnter
	case cp == 0x09:
		k = keys.KeyTab
	case cp == 0x7F, cp == 0x08:
		k = keys.KeyBackspace
	default:
		k = kkpCodepointToKey(rune(cp))
		if k == keys.KeyUnidentified {
			// Treat as printable if it is a valid rune outside
			// the PUA; otherwise drop.
			if cp >= 0x20 && cp < 0x10FFFF {
				text = string(rune(cp))
			} else {
				return
			}
		}
	}

	p.out = append(p.out, KeyEvent{Event: keys.Event{
		Action:    action,
		Key:       k,
		Mods:      mods,
		Text:      text,
		Unshifted: rune(cp),
	}})
}

// parseParams splits a CSI parameter buffer on ';' and parses
// each segment as a decimal integer. Empty segments become 0
// (matching the CSI default-parameter convention). Non-numeric
// segments are treated as 0 too — upstream simply mismatches and
// the sequence gets dropped.
func parseParams(b []byte) []int {
	if len(b) == 0 {
		return nil
	}
	parts := splitByte(b, ';')
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, _ := parseInt(p)
		out = append(out, n)
	}
	return out
}

// splitByte splits b on sep, returning slices that share b's
// backing array. Empty b returns a single empty element rather
// than nil so len(result) is always >= 1 when b is non-nil.
func splitByte(b []byte, sep byte) [][]byte {
	var out [][]byte
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] == sep {
			out = append(out, b[start:i])
			start = i + 1
		}
	}
	out = append(out, b[start:])
	return out
}

// parseInt parses b as a decimal non-negative integer. Returns
// (0, false) if b is empty or contains non-digit characters; the
// ok bit lets callers distinguish "explicit zero" from "no value".
func parseInt(b []byte) (int, bool) {
	if len(b) == 0 {
		return 0, false
	}
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}
