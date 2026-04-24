package keys

import "strings"

// KeyCode is the lookup key for Bindings: a Key + Mods pair with
// side bits dropped and shift normalized for printable ASCII letters
// and digits so that "Shift-a" and the keycap "A" bind the same way.
//
// KeyCode values are produced by Code and consumed by Table.Lookup.
// They are safe to use as map keys and safe to compare with ==.
type KeyCode struct {
	// Key is the physical key after normalization.
	Key Key
	// Mods is the modifier mask after dropping side bits, lock bits,
	// and (for printable ASCII letters/digits) Shift.
	Mods Mods
}

// normalizeMods strips the modifier bits that are never part of a
// binding. Side bits are dropped because bindings are side-insensitive;
// CapsLock/NumLock are state indicators, not modifiers the user holds.
func normalizeMods(m Mods) Mods {
	return m &^ (ModShiftSide | ModCtrlSide | ModAltSide | ModSuperSide |
		ModCapsLock | ModNumLock)
}

// isShiftNormalizedKey reports whether Shift should be folded away
// for k. Printable ASCII letters and digits follow the tmux keycap
// convention: "bind A" and "bind S-a" resolve to the same entry.
// Other keys (F1, Tab, Space, Arrow*, punctuation) keep Shift
// because there is no keycap-level alternative.
func isShiftNormalizedKey(k Key) bool {
	if k >= KeyA && k <= KeyZ {
		return true
	}
	if k >= KeyDigit0 && k <= KeyDigit9 {
		return true
	}
	return false
}

// isModifierKey reports whether k is a standalone modifier key.
// Pressing Shift on its own must never fire a binding: no tmux user
// expects "bind S-something" to mean "pressing Shift alone".
func isModifierKey(k Key) bool {
	switch k {
	case KeyShiftLeft, KeyShiftRight,
		KeyControlLeft, KeyControlRight,
		KeyAltLeft, KeyAltRight,
		KeyMetaLeft, KeyMetaRight:
		return true
	}
	return false
}

// Code derives the KeyCode for e. It returns (KeyCode{}, false) for
// events that should never trigger a binding:
//
//   - releases (Action == ActionRelease);
//   - standalone modifier keys (Shift/Ctrl/Alt/Meta left and right);
//   - events whose Key is KeyUnidentified.
//
// For every other event, Mods is normalized: side bits and lock bits
// are stripped, and Shift is folded away for printable ASCII letters
// and digits so "Shift-a" and "A" match the same binding.
func Code(e Event) (KeyCode, bool) {
	if e.Action == ActionRelease {
		return KeyCode{}, false
	}
	if e.Key == KeyUnidentified {
		return KeyCode{}, false
	}
	if isModifierKey(e.Key) {
		return KeyCode{}, false
	}
	mods := normalizeMods(e.Mods)
	if isShiftNormalizedKey(e.Key) {
		mods &^= ModShift
	}
	return KeyCode{Key: e.Key, Mods: mods}, true
}

// String returns a tmux-style notation for c: modifiers as alphabetical
// single-letter prefixes ("C-" for Ctrl, "M-" for Alt, "S-" for Shift)
// joined by "-" before the key label, with Super rendered as the
// full-word "Super-" prefix. Examples: "a", "C-b", "M-x", "C-M-a",
// "S-F7", "Space". The output is for diagnostics and help text; it is
// not the input grammar for bind-key, which lives in the parser.
func (c KeyCode) String() string {
	if c.Key == KeyUnidentified && c.Mods == 0 {
		return "Unidentified"
	}
	var b strings.Builder
	// Alphabetical order on the letter prefix: C (Ctrl), M (Alt/Meta),
	// S (Shift). Super is spelled out to avoid clashing with Shift.
	if c.Mods&ModCtrl != 0 {
		b.WriteString("C-")
	}
	if c.Mods&ModAlt != 0 {
		b.WriteString("M-")
	}
	if c.Mods&ModShift != 0 {
		b.WriteString("S-")
	}
	if c.Mods&ModSuper != 0 {
		b.WriteString("Super-")
	}
	b.WriteString(c.Key.String())
	return b.String()
}
