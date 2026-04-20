package keys

import (
	"fmt"
	"strings"
)

// Modifier represents keyboard modifier keys as a bitmask.
type Modifier uint8

const (
	ModCtrl  Modifier = 1 << iota // Ctrl key held
	ModAlt                         // Alt/Meta key held
	ModShift                       // Shift key held
)

// KeyCode identifies a key. Positive values are Unicode code points;
// negative values are named constants for special and virtual keys.
type KeyCode int32

// Special key codes. Negative values do not collide with Unicode.
const (
	CodeEnter    KeyCode = -(iota + 1) // Return / Enter
	CodeEscape                          // Escape
	CodeTab                             // Tab
	CodeBackspace                       // Backspace / DEL
	CodeUp                              // Arrow Up
	CodeDown                            // Arrow Down
	CodeLeft                            // Arrow Left
	CodeRight                           // Arrow Right
	CodeHome                            // Home
	CodeEnd                             // End
	CodePageUp                          // Page Up
	CodePageDown                        // Page Down
	CodeInsert                          // Insert
	CodeDelete                          // Delete
	CodeF1                              // Function key F1
	CodeF2                              // Function key F2
	CodeF3                              // Function key F3
	CodeF4                              // Function key F4
	CodeF5                              // Function key F5
	CodeF6                              // Function key F6
	CodeF7                              // Function key F7
	CodeF8                              // Function key F8
	CodeF9                              // Function key F9
	CodeF10                             // Function key F10
	CodeF11                             // Function key F11
	CodeF12                             // Function key F12
	CodeF13                             // Function key F13
	CodeF14                             // Function key F14
	CodeF15                             // Function key F15
	CodeF16                             // Function key F16
	CodeF17                             // Function key F17
	CodeF18                             // Function key F18
	CodeF19                             // Function key F19
	CodeF20                             // Function key F20
	CodeF21                             // Function key F21
	CodeF22                             // Function key F22
	CodeF23                             // Function key F23
	CodeF24                             // Function key F24
	CodePasteStart                      // ESC[200~: start of bracketed paste region
	CodePasteEnd                        // ESC[201~: end of bracketed paste region
	CodeMouse                           // Mouse event (see Key.Mouse)
)

// CodeNone is the zero value: no key / invalid.
const CodeNone KeyCode = 0

// codeNames maps special key codes to their canonical string names.
var codeNames = map[KeyCode]string{
	CodeEnter:     "Enter",
	CodeEscape:    "Escape",
	CodeTab:       "Tab",
	CodeBackspace: "Backspace",
	CodeUp:        "Up",
	CodeDown:      "Down",
	CodeLeft:      "Left",
	CodeRight:     "Right",
	CodeHome:      "Home",
	CodeEnd:       "End",
	CodePageUp:    "PageUp",
	CodePageDown:  "PageDown",
	CodeInsert:    "Insert",
	CodeDelete:    "Delete",
	CodeF1:        "F1",
	CodeF2:        "F2",
	CodeF3:        "F3",
	CodeF4:        "F4",
	CodeF5:        "F5",
	CodeF6:        "F6",
	CodeF7:        "F7",
	CodeF8:        "F8",
	CodeF9:        "F9",
	CodeF10:       "F10",
	CodeF11:       "F11",
	CodeF12:       "F12",
	CodeF13:       "F13",
	CodeF14:       "F14",
	CodeF15:       "F15",
	CodeF16:       "F16",
	CodeF17:       "F17",
	CodeF18:       "F18",
	CodeF19:       "F19",
	CodeF20:       "F20",
	CodeF21:       "F21",
	CodeF22:       "F22",
	CodeF23:       "F23",
	CodeF24:       "F24",
	CodePasteStart: "PasteStart",
	CodePasteEnd:   "PasteEnd",
	CodeMouse:      "Mouse",
	// Space is a printable rune but has a canonical name.
	KeyCode(' '): "Space",
}

// nameToCode maps canonical names to key codes (inverse of codeNames).
var nameToCode = func() map[string]KeyCode {
	m := make(map[string]KeyCode, len(codeNames))
	for code, name := range codeNames {
		m[name] = code
	}
	return m
}()

// MouseAction identifies whether a mouse event is a press, release, or motion.
type MouseAction uint8

const (
	MousePress   MouseAction = iota // Button pressed
	MouseRelease                    // Button released
	MouseMotion                     // Cursor moved (button may or may not be held)
)

// MouseButton identifies which button is involved in a mouse event.
type MouseButton uint8

const (
	MouseNone       MouseButton = iota // No button / button-less motion
	MouseLeft                          // Left button
	MouseMiddle                        // Middle button
	MouseRight                         // Right button
	MouseWheelUp                       // Scroll wheel up
	MouseWheelDown                     // Scroll wheel down
	MouseButton4                       // Extra button 4
	MouseButton5                       // Extra button 5
)

// MouseEvent carries position and button data for mouse key events.
// It is only meaningful when Key.Code == CodeMouse.
type MouseEvent struct {
	Action MouseAction
	Button MouseButton
	Col    int // 0-based column
	Row    int // 0-based row
}

// Key represents a single keyboard or virtual terminal event. It
// carries a code identifying the key, optional modifier flags, and
// (for mouse events) position data.
//
// The zero value is the empty / invalid key (Code == CodeNone).
type Key struct {
	Code  KeyCode
	Mod   Modifier
	Mouse MouseEvent // only meaningful when Code == CodeMouse
}

// String returns the canonical human-readable representation of k.
//
// Format: [C-][M-][S-]<name>
//
// Examples: "C-a", "M-x", "C-M-k", "Enter", "F1", "Space", "a".
// Parse(k.String()) == k for all valid keys.
func (k Key) String() string {
	if k.Code == CodeNone {
		return ""
	}
	var sb strings.Builder
	if k.Mod&ModCtrl != 0 {
		sb.WriteString("C-")
	}
	if k.Mod&ModAlt != 0 {
		sb.WriteString("M-")
	}
	if k.Mod&ModShift != 0 {
		sb.WriteString("S-")
	}
	if name, ok := codeNames[k.Code]; ok {
		sb.WriteString(name)
	} else if k.Code > 0 {
		sb.WriteRune(rune(k.Code))
	} else {
		fmt.Fprintf(&sb, "Unknown(%d)", k.Code)
	}
	return sb.String()
}

// Parse parses a canonical key string into a Key.
//
// The format is [C-][M-][S-]<name-or-rune>, e.g. "C-a", "M-Enter",
// "F1", "Space", "a".
func Parse(s string) (Key, error) {
	var k Key
	for {
		switch {
		case strings.HasPrefix(s, "C-"):
			k.Mod |= ModCtrl
			s = s[2:]
		case strings.HasPrefix(s, "M-"):
			k.Mod |= ModAlt
			s = s[2:]
		case strings.HasPrefix(s, "S-"):
			k.Mod |= ModShift
			s = s[2:]
		default:
			goto done
		}
	}
done:
	if code, ok := nameToCode[s]; ok {
		k.Code = code
		return k, nil
	}
	rs := []rune(s)
	if len(rs) == 1 {
		k.Code = KeyCode(rs[0])
		return k, nil
	}
	return Key{}, fmt.Errorf("keys: unknown key %q", s)
}
