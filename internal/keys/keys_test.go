package keys

import "testing"

// Action constants must match GHOSTTY_KEY_ACTION_* numeric values so
// the pane-side encoder can consume our Events without translation.
func TestActionValues(t *testing.T) {
	cases := []struct {
		name string
		got  Action
		want uint8
	}{
		{"release", ActionRelease, 0},
		{"press", ActionPress, 1},
		{"repeat", ActionRepeat, 2},
	}
	for _, tc := range cases {
		if uint8(tc.got) != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

// Mods bit positions must match GHOSTTY_MODS_* for encoder pass-through.
func TestModsBits(t *testing.T) {
	cases := []struct {
		name string
		got  Mods
		want uint16
	}{
		{"shift", ModShift, 1 << 0},
		{"ctrl", ModCtrl, 1 << 1},
		{"alt", ModAlt, 1 << 2},
		{"super", ModSuper, 1 << 3},
		{"capslock", ModCapsLock, 1 << 4},
		{"numlock", ModNumLock, 1 << 5},
		{"shiftside", ModShiftSide, 1 << 6},
		{"ctrlside", ModCtrlSide, 1 << 7},
		{"altside", ModAltSide, 1 << 8},
		{"superside", ModSuperSide, 1 << 9},
	}
	for _, tc := range cases {
		if uint16(tc.got) != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

// Key constants must match the Ghostty enum's numeric values.
// GhosttyKey is a C enum starting at 0 (UNIDENTIFIED); every later
// enumerator is the previous + 1. We spot-check values scattered
// across the sections rather than enumerating all ~176.
func TestKeyValues(t *testing.T) {
	cases := []struct {
		name string
		got  Key
		want uint16
	}{
		{"unidentified", KeyUnidentified, 0},
		{"backquote (first writing-system key)", KeyBackquote, 1},
		{"digit0", KeyDigit0, 6},
		{"a", KeyA, 20},
		{"z (last letter)", KeyZ, 45},
		{"altleft (first functional key)", KeyAltLeft, 51},
		{"space", KeySpace, 63},
		{"arrowdown (first arrow)", KeyArrowDown, 75},
		{"numpad0", KeyNumpad0, 80},
		{"numpadmemorysubtract", KeyNumpadMemorySubtract, 103},
		{"escape (first function-section)", KeyEscape, 120},
		{"f1", KeyF1, 121},
		{"browserback (first media)", KeyBrowserBack, 151},
		{"paste (last legacy)", KeyPaste, 175},
	}
	for _, tc := range cases {
		if uint16(tc.got) != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

// Code normalizes Shift on ASCII letters: "Shift-a" and the keycap
// "A" must collapse to the same KeyCode.
func TestCodeShiftNormalizesLetter(t *testing.T) {
	got, ok := Code(Event{Action: ActionPress, Key: KeyA, Mods: ModShift})
	if !ok {
		t.Fatal("Code returned ok=false")
	}
	want := KeyCode{Key: KeyA, Mods: 0}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Shift normalization must not strip other modifiers.
func TestCodeShiftNormalizeKeepsCtrl(t *testing.T) {
	got, ok := Code(Event{Action: ActionPress, Key: KeyA, Mods: ModCtrl | ModShift})
	if !ok {
		t.Fatal("Code returned ok=false")
	}
	want := KeyCode{Key: KeyA, Mods: ModCtrl}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Non-letter/digit keys keep Shift: S-F1 must remain distinct from F1.
func TestCodeShiftKeptForFunctionKey(t *testing.T) {
	got, ok := Code(Event{Action: ActionPress, Key: KeyF1, Mods: ModShift})
	if !ok {
		t.Fatal("Code returned ok=false")
	}
	want := KeyCode{Key: KeyF1, Mods: ModShift}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Digits are shift-normalized like letters.
func TestCodeShiftNormalizesDigit(t *testing.T) {
	got, ok := Code(Event{Action: ActionPress, Key: KeyDigit1, Mods: ModShift})
	if !ok {
		t.Fatal("Code returned ok=false")
	}
	want := KeyCode{Key: KeyDigit1, Mods: 0}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Release events never drive bindings.
func TestCodeRejectsRelease(t *testing.T) {
	if _, ok := Code(Event{Action: ActionRelease, Key: KeyA}); ok {
		t.Error("Code accepted a release event")
	}
}

// Repeat events drive bindings (holding down a key must keep firing).
func TestCodeAcceptsRepeat(t *testing.T) {
	got, ok := Code(Event{Action: ActionRepeat, Key: KeyA})
	if !ok {
		t.Fatal("Code rejected a repeat event")
	}
	if got != (KeyCode{Key: KeyA}) {
		t.Errorf("got %+v, want %+v", got, KeyCode{Key: KeyA})
	}
}

// Standalone modifier keys never drive bindings.
func TestCodeRejectsStandaloneModifiers(t *testing.T) {
	for _, k := range []Key{
		KeyShiftLeft, KeyShiftRight,
		KeyControlLeft, KeyControlRight,
		KeyAltLeft, KeyAltRight,
		KeyMetaLeft, KeyMetaRight,
	} {
		if _, ok := Code(Event{Action: ActionPress, Key: k}); ok {
			t.Errorf("Code accepted standalone modifier %s", k)
		}
	}
}

// KeyUnidentified never drives bindings.
func TestCodeRejectsUnidentified(t *testing.T) {
	if _, ok := Code(Event{Action: ActionPress, Key: KeyUnidentified}); ok {
		t.Error("Code accepted Unidentified")
	}
}

// Side bits are stripped. The user pressing right-Ctrl vs left-Ctrl
// must land on the same binding.
func TestCodeStripsSideBits(t *testing.T) {
	got, ok := Code(Event{Action: ActionPress, Key: KeyA, Mods: ModCtrl | ModCtrlSide})
	if !ok {
		t.Fatal("Code returned ok=false")
	}
	want := KeyCode{Key: KeyA, Mods: ModCtrl}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got.Mods&ModCtrlSide != 0 {
		t.Error("ModCtrlSide survived normalization")
	}
}

// Lock bits are state indicators, not modifiers — they must be stripped.
func TestCodeStripsLockBits(t *testing.T) {
	got, ok := Code(Event{Action: ActionPress, Key: KeyArrowDown, Mods: ModCapsLock | ModNumLock | ModCtrl})
	if !ok {
		t.Fatal("Code returned ok=false")
	}
	want := KeyCode{Key: KeyArrowDown, Mods: ModCtrl}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// KeyCode.String samples covering plain letters, Ctrl, function keys
// with Shift, special keys, and multi-modifier bindings.
func TestKeyCodeStringSamples(t *testing.T) {
	cases := []struct {
		code KeyCode
		want string
	}{
		{KeyCode{Key: KeyA}, "a"},
		{KeyCode{Key: KeyA, Mods: ModCtrl}, "C-a"},
		{KeyCode{Key: KeyF1, Mods: ModShift}, "S-F1"},
		{KeyCode{Key: KeySpace}, "Space"},
		{KeyCode{Key: KeyA, Mods: ModCtrl | ModAlt}, "C-M-a"},
		{KeyCode{Key: KeyX, Mods: ModAlt}, "M-x"},
		{KeyCode{Key: KeyA, Mods: ModCtrl | ModAlt | ModShift}, "C-M-S-a"},
		{KeyCode{Key: KeyK, Mods: ModSuper}, "Super-k"},
		{KeyCode{Key: KeyArrowDown, Mods: ModCtrl}, "C-ArrowDown"},
	}
	for _, tc := range cases {
		if got := tc.code.String(); got != tc.want {
			t.Errorf("%+v.String() = %q, want %q", tc.code, got, tc.want)
		}
	}
}

// Table Bind/Unbind/Lookup round-trip, rebind returns previous,
// Unbind returns the removed binding and subsequent Lookup is nil.
func TestTableBindLookupUnbind(t *testing.T) {
	tbl := NewTable("root")
	code := KeyCode{Key: KeyA, Mods: ModCtrl}

	if got := tbl.Lookup(code); got != nil {
		t.Fatalf("empty table Lookup = %+v, want nil", got)
	}

	first := &Binding{Key: code, Argv: []string{"send-prefix"}, Note: "one"}
	if prev := tbl.Bind(first); prev != nil {
		t.Errorf("first Bind returned %+v, want nil", prev)
	}
	if got := tbl.Lookup(code); got != first {
		t.Errorf("Lookup after Bind = %+v, want %+v", got, first)
	}

	second := &Binding{Key: code, Argv: []string{"copy-mode"}, Note: "two"}
	if prev := tbl.Bind(second); prev != first {
		t.Errorf("rebind returned %+v, want %+v", prev, first)
	}
	if got := tbl.Lookup(code); got != second {
		t.Errorf("Lookup after rebind = %+v, want %+v", got, second)
	}

	if got := tbl.Unbind(code); got != second {
		t.Errorf("Unbind returned %+v, want %+v", got, second)
	}
	if got := tbl.Lookup(code); got != nil {
		t.Errorf("Lookup after Unbind = %+v, want nil", got)
	}
	if got := tbl.Unbind(code); got != nil {
		t.Errorf("second Unbind returned %+v, want nil", got)
	}
}

// Zero-value Tables must behave like empty ones without panicking.
func TestTableZeroValue(t *testing.T) {
	var tbl Table
	code := KeyCode{Key: KeyA}
	if got := tbl.Lookup(code); got != nil {
		t.Errorf("zero Table Lookup = %+v, want nil", got)
	}
	if got := tbl.Unbind(code); got != nil {
		t.Errorf("zero Table Unbind = %+v, want nil", got)
	}
	if prev := tbl.Bind(&Binding{Key: code, Argv: []string{"noop"}}); prev != nil {
		t.Errorf("zero Table Bind returned %+v, want nil", prev)
	}
	if got := tbl.Lookup(code); got == nil {
		t.Error("Lookup after Bind on zero Table returned nil")
	}
}

// Key.String spot-checks: a printable letter, a named functional key,
// a punctuation key, and an out-of-range value all produce useful
// text.
func TestKeyString(t *testing.T) {
	cases := []struct {
		key  Key
		want string
	}{
		{KeyA, "a"},
		{KeyArrowDown, "ArrowDown"},
		{KeySemicolon, ";"},
		{KeyNumpadEnter, "NumpadEnter"},
		{Key(9999), "Unidentified"},
	}
	for _, tc := range cases {
		if got := tc.key.String(); got != tc.want {
			t.Errorf("Key(%d).String() = %q, want %q", tc.key, got, tc.want)
		}
	}
}
