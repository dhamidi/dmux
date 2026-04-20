package keys

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// Decoder reads raw bytes from a terminal's stdin and yields [Key]
// events. It handles:
//   - Printable Unicode characters and ASCII control codes
//   - xterm-style escape sequences (CSI, SS3)
//   - CSI u keyboard protocol
//   - Kitty keyboard protocol (superset of CSI u)
//   - Bracketed paste sequences (ESC[200~ / ESC[201~)
//   - X10, normal, and SGR mouse sequences
//
// Decoder is not safe for concurrent use by multiple goroutines.
type Decoder struct {
	r *bufio.Reader
}

// NewDecoder wraps r in a Decoder. Use [Decoder.Next] to read events.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

// Next reads and returns the next key event from the underlying
// reader. It returns [io.EOF] when the reader is exhausted.
//
// Decoder.Next is a pure function of the input bytes: it has no
// side effects other than advancing the read position and returning
// a Key.
func (d *Decoder) Next() (Key, error) {
	b, err := d.r.ReadByte()
	if err != nil {
		return Key{}, err
	}

	switch {
	case b == 0x00:
		// NUL: Ctrl+Space
		return Key{Code: KeyCode(' '), Mod: ModCtrl}, nil

	case b == 0x1B:
		// ESC: start of escape sequence or standalone Escape
		return d.parseEscape()

	case b >= 0x01 && b <= 0x1A:
		// Ctrl+A … Ctrl+Z (excluding ESC=0x1B handled above)
		switch b {
		case 0x08:
			// Ctrl+H is traditionally Backspace
			return Key{Code: CodeBackspace}, nil
		case 0x09:
			// Ctrl+I = Tab
			return Key{Code: CodeTab}, nil
		case 0x0A, 0x0D:
			// Ctrl+J (LF) and Ctrl+M (CR) both mean Enter
			return Key{Code: CodeEnter}, nil
		default:
			return Key{Code: KeyCode('a' + b - 1), Mod: ModCtrl}, nil
		}

	case b == 0x1C:
		return Key{Code: '\\', Mod: ModCtrl}, nil
	case b == 0x1D:
		return Key{Code: ']', Mod: ModCtrl}, nil
	case b == 0x1E:
		return Key{Code: '^', Mod: ModCtrl}, nil
	case b == 0x1F:
		return Key{Code: '_', Mod: ModCtrl}, nil

	case b == 0x7F:
		// DEL: Backspace
		return Key{Code: CodeBackspace}, nil

	default:
		// Printable ASCII or start of multi-byte UTF-8
		return d.readRuneFrom(b)
	}
}

// readRuneFrom completes a UTF-8 rune whose first byte is first.
func (d *Decoder) readRuneFrom(first byte) (Key, error) {
	if first < 0x80 {
		return Key{Code: KeyCode(first)}, nil
	}
	// Multi-byte: unread the first byte so ReadRune can reassemble it.
	if err := d.r.UnreadByte(); err != nil {
		return Key{}, err
	}
	r, _, err := d.r.ReadRune()
	if err != nil {
		return Key{}, err
	}
	return Key{Code: KeyCode(r)}, nil
}

// parseEscape handles the byte(s) after an ESC (0x1B).
func (d *Decoder) parseEscape() (Key, error) {
	next, err := d.r.Peek(1)
	if err != nil {
		// Nothing follows: standalone Escape key.
		return Key{Code: CodeEscape}, nil
	}

	switch next[0] {
	case '[':
		d.r.ReadByte() //nolint:errcheck // already peeked
		return d.parseCSI()

	case 'O':
		d.r.ReadByte() //nolint:errcheck
		return d.parseSS3()

	case 0x1B:
		// ESC ESC → Escape (leave second ESC unconsumed)
		return Key{Code: CodeEscape}, nil

	default:
		b, _ := d.r.ReadByte()
		// Alt + the following key
		inner, err := d.decodeAltKey(b)
		if err != nil {
			return Key{}, err
		}
		inner.Mod |= ModAlt
		return inner, nil
	}
}

// decodeAltKey decodes the key that follows an ESC prefix (Alt key).
func (d *Decoder) decodeAltKey(b byte) (Key, error) {
	switch {
	case b == 0x08 || b == 0x7F:
		return Key{Code: CodeBackspace}, nil
	case b == 0x09:
		return Key{Code: CodeTab}, nil
	case b == 0x0A || b == 0x0D:
		return Key{Code: CodeEnter}, nil
	case b >= 0x01 && b <= 0x1A:
		return Key{Code: KeyCode('a' + b - 1), Mod: ModCtrl}, nil
	default:
		return d.readRuneFrom(b)
	}
}

// parseCSI reads a CSI (ESC[) sequence and returns the decoded Key.
func (d *Decoder) parseCSI() (Key, error) {
	// Collect all parameter and intermediate bytes until the final byte.
	// Parameter bytes: 0x30–0x3F (digits, ;, :, <, >, ?, !)
	// Intermediate bytes: 0x20–0x2F (rare)
	// Final byte: 0x40–0x7E
	var buf []byte
	for {
		b, err := d.r.ReadByte()
		if err != nil {
			return Key{}, err
		}
		if b >= 0x40 && b <= 0x7E {
			return d.dispatchCSI(string(buf), b)
		}
		buf = append(buf, b)
	}
}

// parseSS3 reads an SS3 (ESC O) sequence and returns the decoded Key.
func (d *Decoder) parseSS3() (Key, error) {
	b, err := d.r.ReadByte()
	if err != nil {
		return Key{}, err
	}
	switch b {
	case 'A':
		return Key{Code: CodeUp}, nil
	case 'B':
		return Key{Code: CodeDown}, nil
	case 'C':
		return Key{Code: CodeRight}, nil
	case 'D':
		return Key{Code: CodeLeft}, nil
	case 'H':
		return Key{Code: CodeHome}, nil
	case 'F':
		return Key{Code: CodeEnd}, nil
	case 'P':
		return Key{Code: CodeF1}, nil
	case 'Q':
		return Key{Code: CodeF2}, nil
	case 'R':
		return Key{Code: CodeF3}, nil
	case 'S':
		return Key{Code: CodeF4}, nil
	default:
		return Key{Code: CodeNone}, nil
	}
}

// dispatchCSI routes a complete CSI sequence to the appropriate handler.
// It is a method on Decoder so it can read additional bytes for X10 mouse.
func (d *Decoder) dispatchCSI(params string, final byte) (Key, error) {
	switch final {
	case 'A':
		return keyWithCSIArrowMods(CodeUp, params)
	case 'B':
		return keyWithCSIArrowMods(CodeDown, params)
	case 'C':
		return keyWithCSIArrowMods(CodeRight, params)
	case 'D':
		return keyWithCSIArrowMods(CodeLeft, params)
	case 'H':
		return keyWithCSIArrowMods(CodeHome, params)
	case 'F':
		return keyWithCSIArrowMods(CodeEnd, params)
	case 'P':
		return keyWithCSIArrowMods(CodeF1, params)
	case 'Q':
		return keyWithCSIArrowMods(CodeF2, params)
	case 'R':
		return keyWithCSIArrowMods(CodeF3, params)
	case 'S':
		return keyWithCSIArrowMods(CodeF4, params)
	case '~':
		return dispatchCSITilde(params)
	case 'u':
		return dispatchCSIU(params)
	case 'M':
		if strings.HasPrefix(params, "<") {
			return parseSGRMouse(params[1:], false)
		}
		if params == "" {
			// X10 mouse: read the 3 trailing bytes
			var buf [3]byte
			if _, err := io.ReadFull(d.r, buf[:]); err != nil {
				return Key{}, err
			}
			return ParseX10Mouse(buf), nil
		}
		return Key{Code: CodeNone}, nil
	case 'm':
		if strings.HasPrefix(params, "<") {
			return parseSGRMouse(params[1:], true)
		}
		return Key{Code: CodeNone}, nil
	default:
		return Key{Code: CodeNone}, nil
	}
}

// keyWithCSIArrowMods extracts modifiers from a CSI arrow/nav sequence.
// For ESC[A the params are empty → no modifiers.
// For ESC[1;5A the params are "1;5" → modifier param is "5".
func keyWithCSIArrowMods(code KeyCode, params string) (Key, error) {
	mod := parseCSIModParam(secondParam(params))
	return Key{Code: code, Mod: mod}, nil
}

// secondParam returns the second semicolon-separated field from params,
// or "" if there is none.
func secondParam(params string) string {
	if idx := strings.Index(params, ";"); idx >= 0 {
		rest := params[idx+1:]
		if end := strings.Index(rest, ";"); end >= 0 {
			return rest[:end]
		}
		return rest
	}
	return ""
}

// parseCSIModParam decodes the xterm modifier parameter (2–8) into Modifier.
// mod_param = (shift<<0 | alt<<1 | ctrl<<2) + 1
func parseCSIModParam(s string) Modifier {
	if s == "" || s == "1" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 2 {
		return 0
	}
	bits := n - 1
	var m Modifier
	if bits&1 != 0 {
		m |= ModShift
	}
	if bits&2 != 0 {
		m |= ModAlt
	}
	if bits&4 != 0 {
		m |= ModCtrl
	}
	return m
}

// dispatchCSITilde decodes ESC[N~ and ESC[N;modM~ sequences.
func dispatchCSITilde(params string) (Key, error) {
	parts := strings.SplitN(params, ";", 2)
	num, _ := strconv.Atoi(parts[0])
	var modPart string
	if len(parts) >= 2 {
		modPart = parts[1]
	}
	mod := parseCSIModParam(modPart)

	var code KeyCode
	switch num {
	case 1, 7:
		code = CodeHome
	case 2:
		code = CodeInsert
	case 3:
		code = CodeDelete
	case 4, 8:
		code = CodeEnd
	case 5:
		code = CodePageUp
	case 6:
		code = CodePageDown
	case 11:
		code = CodeF1
	case 12:
		code = CodeF2
	case 13:
		code = CodeF3
	case 14:
		code = CodeF4
	case 15:
		code = CodeF5
	case 17:
		code = CodeF6
	case 18:
		code = CodeF7
	case 19:
		code = CodeF8
	case 20:
		code = CodeF9
	case 21:
		code = CodeF10
	case 23:
		code = CodeF11
	case 24:
		code = CodeF12
	case 25:
		code = CodeF13
	case 26:
		code = CodeF14
	case 28:
		code = CodeF15
	case 29:
		code = CodeF16
	case 31:
		code = CodeF17
	case 32:
		code = CodeF18
	case 33:
		code = CodeF19
	case 34:
		code = CodeF20
	case 200:
		return Key{Code: CodePasteStart}, nil
	case 201:
		return Key{Code: CodePasteEnd}, nil
	default:
		return Key{Code: CodeNone}, nil
	}
	return Key{Code: code, Mod: mod}, nil
}

// dispatchCSIU decodes CSI u (and Kitty keyboard protocol) sequences.
//
// Format: ESC [ codepoint ; modifiers : eventtype u
// Modifiers: (shift=1, alt=2, ctrl=4, super=8, ...) + 1
// Event type (Kitty): 1=press, 2=repeat, 3=release (we accept all)
func dispatchCSIU(params string) (Key, error) {
	// Split on ";" to get codepoint and modifier fields.
	parts := strings.SplitN(params, ";", 2)
	cp, err := strconv.Atoi(parts[0])
	if err != nil {
		return Key{Code: CodeNone}, nil
	}

	var mod Modifier
	if len(parts) >= 2 {
		modStr := parts[1]
		// Kitty event type follows ":" — strip it.
		if idx := strings.Index(modStr, ":"); idx >= 0 {
			modStr = modStr[:idx]
		}
		mod = parseCSIUModParam(modStr)
	}

	code := codepointToCode(rune(cp))
	return Key{Code: code, Mod: mod}, nil
}

// parseCSIUModParam decodes a CSI u modifier parameter.
// Same encoding as xterm: (shift=1, alt=2, ctrl=4) + 1.
func parseCSIUModParam(s string) Modifier {
	if s == "" || s == "1" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 2 {
		return 0
	}
	bits := n - 1
	var m Modifier
	if bits&1 != 0 {
		m |= ModShift
	}
	if bits&2 != 0 {
		m |= ModAlt
	}
	if bits&4 != 0 {
		m |= ModCtrl
	}
	return m
}

// codepointToCode maps a Unicode code point (as used in CSI u) to a KeyCode.
func codepointToCode(r rune) KeyCode {
	switch r {
	case 13:
		return CodeEnter
	case 9:
		return CodeTab
	case 27:
		return CodeEscape
	case 127:
		return CodeBackspace
	default:
		return KeyCode(r)
	}
}

// ParseX10Mouse decodes an X10 mouse sequence from three raw bytes.
// The caller must have already consumed the ESC[M prefix; bytes is
// the three bytes that follow: button, col, row (each +0x20).
//
// This is exported so callers can feed pre-read bytes directly when
// the ESC[M sequence was already partially consumed.
func ParseX10Mouse(bytes [3]byte) Key {
	raw := int(bytes[0]) - 0x20
	col := int(bytes[1]) - 0x20 - 1
	row := int(bytes[2]) - 0x20 - 1

	var action MouseAction
	if raw&3 == 3 {
		action = MouseRelease
	} else if raw&32 != 0 {
		action = MouseMotion
	} else {
		action = MousePress
	}

	button := rawToButton(raw)
	mod := rawToMod(raw)

	return Key{
		Code: CodeMouse,
		Mod:  mod,
		Mouse: MouseEvent{
			Action: action,
			Button: button,
			Col:    col,
			Row:    row,
		},
	}
}

// parseSGRMouse decodes an SGR mouse sequence (ESC[<...M or ESC[<...m).
// params is the content after '<'; release is true for the 'm' final byte.
func parseSGRMouse(params string, release bool) (Key, error) {
	parts := strings.SplitN(params, ";", 3)
	if len(parts) < 3 {
		return Key{Code: CodeNone}, nil
	}
	btn, _ := strconv.Atoi(parts[0])
	col, _ := strconv.Atoi(parts[1])
	row, _ := strconv.Atoi(parts[2])

	var action MouseAction
	switch {
	case release:
		action = MouseRelease
	case btn&32 != 0:
		action = MouseMotion
	default:
		action = MousePress
	}

	button := rawToButton(btn)
	mod := rawToMod(btn)

	return Key{
		Code: CodeMouse,
		Mod:  mod,
		Mouse: MouseEvent{
			Action: action,
			Button: button,
			Col:    col - 1,
			Row:    row - 1,
		},
	}, nil
}

// rawToButton maps a raw mouse button byte to MouseButton.
func rawToButton(raw int) MouseButton {
	if raw&64 != 0 {
		// Wheel event
		if raw&1 != 0 {
			return MouseWheelDown
		}
		return MouseWheelUp
	}
	switch raw & 3 {
	case 0:
		return MouseLeft
	case 1:
		return MouseMiddle
	case 2:
		return MouseRight
	default:
		return MouseNone
	}
}

// rawToMod maps raw mouse modifier bits to Modifier.
func rawToMod(raw int) Modifier {
	var m Modifier
	if raw&4 != 0 {
		m |= ModShift
	}
	if raw&8 != 0 {
		m |= ModAlt
	}
	if raw&16 != 0 {
		m |= ModCtrl
	}
	return m
}
