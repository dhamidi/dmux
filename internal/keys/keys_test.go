package keys_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
)

// ---- helper ----------------------------------------------------------------

func decode(t *testing.T, input []byte) []keys.Key {
	t.Helper()
	d := keys.NewDecoder(bytes.NewReader(input))
	var out []keys.Key
	for {
		k, err := d.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Next() error: %v", err)
		}
		out = append(out, k)
	}
	return out
}

func decodeOne(t *testing.T, input []byte) keys.Key {
	t.Helper()
	got := decode(t, input)
	if len(got) != 1 {
		t.Fatalf("expected 1 key, got %d: %v", len(got), got)
	}
	return got[0]
}

func assertKey(t *testing.T, got, want keys.Key) {
	t.Helper()
	if got != want {
		t.Errorf("got %+v (%s), want %+v (%s)", got, got.String(), want, want.String())
	}
}

// ---- Parse / String round-trip ---------------------------------------------

func TestParseString_RoundTrip(t *testing.T) {
	cases := []string{
		"a", "A", "z", "Z", "0", "9", "@", "~",
		"Enter", "Escape", "Tab", "Backspace", "Space",
		"Up", "Down", "Left", "Right",
		"Home", "End", "PageUp", "PageDown", "Insert", "Delete",
		"F1", "F2", "F3", "F4", "F5", "F6",
		"F7", "F8", "F9", "F10", "F11", "F12",
		"C-a", "M-a", "C-M-a", "S-a",
		"C-Enter", "M-Up", "C-M-F1",
	}
	for _, s := range cases {
		k, err := keys.Parse(s)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", s, err)
			continue
		}
		if got := k.String(); got != s {
			t.Errorf("Parse(%q).String() = %q, want %q", s, got, s)
		}
	}
}

func TestParseUnknown(t *testing.T) {
	_, err := keys.Parse("NotAKey")
	if err == nil {
		t.Error("Parse(\"NotAKey\") expected error, got nil")
	}
}

// ---- ASCII keys ------------------------------------------------------------

func TestDecoder_ASCIIPrintable(t *testing.T) {
	cases := []struct {
		in   byte
		want keys.Key
	}{
		{'a', mustParse(t, "a")},
		{'z', mustParse(t, "z")},
		{'A', mustParse(t, "A")},
		{'0', mustParse(t, "0")},
		{' ', mustParse(t, "Space")},
		{'!', mustParse(t, "!")},
	}
	for _, c := range cases {
		got := decodeOne(t, []byte{c.in})
		assertKey(t, got, c.want)
	}
}

func TestDecoder_CtrlKeys(t *testing.T) {
	cases := []struct {
		in   byte
		want keys.Key
	}{
		{0x01, mustParse(t, "C-a")},
		{0x02, mustParse(t, "C-b")},
		{0x05, mustParse(t, "C-e")},
		{0x0B, mustParse(t, "C-k")},
		{0x0C, mustParse(t, "C-l")},
		{0x0E, mustParse(t, "C-n")},
		{0x10, mustParse(t, "C-p")},
		{0x15, mustParse(t, "C-u")},
		{0x17, mustParse(t, "C-w")},
		{0x1A, mustParse(t, "C-z")},
	}
	for _, c := range cases {
		got := decodeOne(t, []byte{c.in})
		assertKey(t, got, c.want)
	}
}

func TestDecoder_SpecialASCII(t *testing.T) {
	assertKey(t, decodeOne(t, []byte{0x09}), mustParse(t, "Tab"))
	assertKey(t, decodeOne(t, []byte{0x0D}), mustParse(t, "Enter"))
	assertKey(t, decodeOne(t, []byte{0x0A}), mustParse(t, "Enter"))
	assertKey(t, decodeOne(t, []byte{0x7F}), mustParse(t, "Backspace"))
	assertKey(t, decodeOne(t, []byte{0x08}), mustParse(t, "Backspace"))
}

func TestDecoder_StandaloneEscape(t *testing.T) {
	assertKey(t, decodeOne(t, []byte{0x1B}), mustParse(t, "Escape"))
}

func TestDecoder_AltKey(t *testing.T) {
	// ESC + 'x' → M-x
	got := decodeOne(t, []byte{0x1B, 'x'})
	assertKey(t, got, mustParse(t, "M-x"))
}

func TestDecoder_AltEnter(t *testing.T) {
	// ESC + CR → M-Enter
	got := decodeOne(t, []byte{0x1B, 0x0D})
	want := keys.Key{Code: keys.CodeEnter, Mod: keys.ModAlt}
	assertKey(t, got, want)
}

func TestDecoder_UTF8(t *testing.T) {
	// '€' is U+20AC, encoded as 0xE2 0x82 0xAC in UTF-8
	got := decodeOne(t, []byte{0xE2, 0x82, 0xAC})
	want := keys.Key{Code: keys.KeyCode(0x20AC)}
	assertKey(t, got, want)
}

// ---- xterm escape sequences ------------------------------------------------

func TestDecoder_ArrowKeys(t *testing.T) {
	cases := []struct {
		in   []byte
		want keys.Key
	}{
		{[]byte{0x1B, '[', 'A'}, mustParse(t, "Up")},
		{[]byte{0x1B, '[', 'B'}, mustParse(t, "Down")},
		{[]byte{0x1B, '[', 'C'}, mustParse(t, "Right")},
		{[]byte{0x1B, '[', 'D'}, mustParse(t, "Left")},
	}
	for _, c := range cases {
		got := decodeOne(t, c.in)
		assertKey(t, got, c.want)
	}
}

func TestDecoder_ArrowKeysWithMods(t *testing.T) {
	// ESC[1;5A = Ctrl+Up
	got := decodeOne(t, []byte{0x1B, '[', '1', ';', '5', 'A'})
	assertKey(t, got, mustParse(t, "C-Up"))
}

func TestDecoder_NavKeys(t *testing.T) {
	cases := []struct {
		in   []byte
		want keys.Key
	}{
		{[]byte{0x1B, '[', 'H'}, mustParse(t, "Home")},
		{[]byte{0x1B, '[', 'F'}, mustParse(t, "End")},
		{[]byte{0x1B, '[', '2', '~'}, mustParse(t, "Insert")},
		{[]byte{0x1B, '[', '3', '~'}, mustParse(t, "Delete")},
		{[]byte{0x1B, '[', '5', '~'}, mustParse(t, "PageUp")},
		{[]byte{0x1B, '[', '6', '~'}, mustParse(t, "PageDown")},
		{[]byte{0x1B, '[', '1', '~'}, mustParse(t, "Home")},
		{[]byte{0x1B, '[', '4', '~'}, mustParse(t, "End")},
	}
	for _, c := range cases {
		got := decodeOne(t, c.in)
		assertKey(t, got, c.want)
	}
}

func TestDecoder_FunctionKeys_CSI(t *testing.T) {
	cases := []struct {
		in   []byte
		code keys.KeyCode
	}{
		{[]byte{0x1B, '[', '1', '1', '~'}, keys.CodeF1},
		{[]byte{0x1B, '[', '1', '2', '~'}, keys.CodeF2},
		{[]byte{0x1B, '[', '1', '3', '~'}, keys.CodeF3},
		{[]byte{0x1B, '[', '1', '4', '~'}, keys.CodeF4},
		{[]byte{0x1B, '[', '1', '5', '~'}, keys.CodeF5},
		{[]byte{0x1B, '[', '1', '7', '~'}, keys.CodeF6},
		{[]byte{0x1B, '[', '1', '8', '~'}, keys.CodeF7},
		{[]byte{0x1B, '[', '1', '9', '~'}, keys.CodeF8},
		{[]byte{0x1B, '[', '2', '0', '~'}, keys.CodeF9},
		{[]byte{0x1B, '[', '2', '1', '~'}, keys.CodeF10},
		{[]byte{0x1B, '[', '2', '3', '~'}, keys.CodeF11},
		{[]byte{0x1B, '[', '2', '4', '~'}, keys.CodeF12},
	}
	for _, c := range cases {
		got := decodeOne(t, c.in)
		if got.Code != c.code {
			t.Errorf("input %v: got code %d, want %d", c.in, got.Code, c.code)
		}
	}
}

func TestDecoder_FunctionKeys_SS3(t *testing.T) {
	cases := []struct {
		in   []byte
		code keys.KeyCode
	}{
		{[]byte{0x1B, 'O', 'P'}, keys.CodeF1},
		{[]byte{0x1B, 'O', 'Q'}, keys.CodeF2},
		{[]byte{0x1B, 'O', 'R'}, keys.CodeF3},
		{[]byte{0x1B, 'O', 'S'}, keys.CodeF4},
		{[]byte{0x1B, 'O', 'A'}, keys.CodeUp},
		{[]byte{0x1B, 'O', 'B'}, keys.CodeDown},
		{[]byte{0x1B, 'O', 'C'}, keys.CodeRight},
		{[]byte{0x1B, 'O', 'D'}, keys.CodeLeft},
	}
	for _, c := range cases {
		got := decodeOne(t, c.in)
		if got.Code != c.code {
			t.Errorf("input %v: got code %d, want %d", c.in, got.Code, c.code)
		}
	}
}

// ---- CSI u / Kitty keyboard protocol ---------------------------------------

func TestDecoder_CSIU_Basic(t *testing.T) {
	// ESC[97u → 'a' (codepoint 97, no modifier)
	got := decodeOne(t, []byte{0x1B, '[', '9', '7', 'u'})
	want := keys.Key{Code: keys.KeyCode('a')}
	assertKey(t, got, want)
}

func TestDecoder_CSIU_WithCtrl(t *testing.T) {
	// ESC[97;5u → Ctrl+a (mods=5 means ctrl: 5-1=4=ctrl)
	got := decodeOne(t, []byte{0x1B, '[', '9', '7', ';', '5', 'u'})
	want := keys.Key{Code: keys.KeyCode('a'), Mod: keys.ModCtrl}
	assertKey(t, got, want)
}

func TestDecoder_CSIU_WithAlt(t *testing.T) {
	// ESC[97;3u → Alt+a (mods=3: 3-1=2=alt)
	got := decodeOne(t, []byte{0x1B, '[', '9', '7', ';', '3', 'u'})
	want := keys.Key{Code: keys.KeyCode('a'), Mod: keys.ModAlt}
	assertKey(t, got, want)
}

func TestDecoder_CSIU_Enter(t *testing.T) {
	// ESC[13u → Enter (codepoint 13)
	got := decodeOne(t, []byte{0x1B, '[', '1', '3', 'u'})
	want := keys.Key{Code: keys.CodeEnter}
	assertKey(t, got, want)
}

func TestDecoder_Kitty_WithEventType(t *testing.T) {
	// ESC[97;5:1u → Ctrl+a press (Kitty event type 1=press)
	got := decodeOne(t, []byte{0x1B, '[', '9', '7', ';', '5', ':', '1', 'u'})
	want := keys.Key{Code: keys.KeyCode('a'), Mod: keys.ModCtrl}
	assertKey(t, got, want)
}

func TestDecoder_Kitty_Release(t *testing.T) {
	// ESC[97;1:3u → a release (Kitty event type 3=release); we accept it
	got := decodeOne(t, []byte{0x1B, '[', '9', '7', ';', '1', ':', '3', 'u'})
	want := keys.Key{Code: keys.KeyCode('a')}
	assertKey(t, got, want)
}

// ---- Bracketed paste -------------------------------------------------------

func TestDecoder_BracketedPaste(t *testing.T) {
	// ESC[200~ followed by ESC[201~
	input := append(
		[]byte{0x1B, '[', '2', '0', '0', '~'},
		[]byte{0x1B, '[', '2', '0', '1', '~'}...,
	)
	got := decode(t, input)
	if len(got) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(got))
	}
	if got[0].Code != keys.CodePasteStart {
		t.Errorf("first key: got code %d, want CodePasteStart", got[0].Code)
	}
	if got[1].Code != keys.CodePasteEnd {
		t.Errorf("second key: got code %d, want CodePasteEnd", got[1].Code)
	}
}

func TestDecoder_BracketedPaste_WithContent(t *testing.T) {
	// ESC[200~ + "hi" + ESC[201~
	input := []byte{0x1B, '[', '2', '0', '0', '~',
		'h', 'i',
		0x1B, '[', '2', '0', '1', '~'}
	got := decode(t, input)
	if len(got) != 4 {
		t.Fatalf("expected 4 keys (PasteStart, h, i, PasteEnd), got %d: %v", len(got), got)
	}
	if got[0].Code != keys.CodePasteStart {
		t.Errorf("[0]: want CodePasteStart, got %v", got[0])
	}
	if got[1].Code != keys.KeyCode('h') {
		t.Errorf("[1]: want 'h', got %v", got[1])
	}
	if got[2].Code != keys.KeyCode('i') {
		t.Errorf("[2]: want 'i', got %v", got[2])
	}
	if got[3].Code != keys.CodePasteEnd {
		t.Errorf("[3]: want CodePasteEnd, got %v", got[3])
	}
}

// ---- Mouse sequences -------------------------------------------------------

func TestDecoder_SGRMouse_Press(t *testing.T) {
	// ESC[<0;10;5M → left button press at col=10 row=5
	input := []byte("\x1b[<0;10;5M")
	got := decodeOne(t, input)
	if got.Code != keys.CodeMouse {
		t.Fatalf("got code %d, want CodeMouse", got.Code)
	}
	if got.Mouse.Action != keys.MousePress {
		t.Errorf("action: got %d, want MousePress", got.Mouse.Action)
	}
	if got.Mouse.Button != keys.MouseLeft {
		t.Errorf("button: got %d, want MouseLeft", got.Mouse.Button)
	}
	if got.Mouse.Col != 9 { // 10-1=9
		t.Errorf("col: got %d, want 9", got.Mouse.Col)
	}
	if got.Mouse.Row != 4 { // 5-1=4
		t.Errorf("row: got %d, want 4", got.Mouse.Row)
	}
}

func TestDecoder_SGRMouse_Release(t *testing.T) {
	// ESC[<0;10;5m → left button release
	input := []byte("\x1b[<0;10;5m")
	got := decodeOne(t, input)
	if got.Code != keys.CodeMouse {
		t.Fatalf("got code %d, want CodeMouse", got.Code)
	}
	if got.Mouse.Action != keys.MouseRelease {
		t.Errorf("action: got %d, want MouseRelease", got.Mouse.Action)
	}
}

func TestDecoder_SGRMouse_WheelUp(t *testing.T) {
	// ESC[<64;1;1M → wheel up
	input := []byte("\x1b[<64;1;1M")
	got := decodeOne(t, input)
	if got.Code != keys.CodeMouse {
		t.Fatalf("got code %d, want CodeMouse", got.Code)
	}
	if got.Mouse.Button != keys.MouseWheelUp {
		t.Errorf("button: got %d, want MouseWheelUp", got.Mouse.Button)
	}
}

func TestDecoder_SGRMouse_WithCtrl(t *testing.T) {
	// ESC[<16;1;1M → left button press with Ctrl
	input := []byte("\x1b[<16;1;1M")
	got := decodeOne(t, input)
	if got.Code != keys.CodeMouse {
		t.Fatalf("got code %d, want CodeMouse", got.Code)
	}
	if got.Mod&keys.ModCtrl == 0 {
		t.Errorf("expected ModCtrl to be set, got mod=%d", got.Mod)
	}
}

func TestDecoder_X10Mouse(t *testing.T) {
	// ESC[M + button(0+0x20) + col(5+0x20) + row(3+0x20)
	input := []byte{0x1B, '[', 'M', 0x20, 0x25, 0x23}
	got := decodeOne(t, input)
	if got.Code != keys.CodeMouse {
		t.Fatalf("got code %d, want CodeMouse", got.Code)
	}
	if got.Mouse.Action != keys.MousePress {
		t.Errorf("action: got %d, want MousePress", got.Mouse.Action)
	}
	if got.Mouse.Button != keys.MouseLeft {
		t.Errorf("button: got %d, want MouseLeft", got.Mouse.Button)
	}
	if got.Mouse.Col != 4 { // 5-1=4
		t.Errorf("col: got %d, want 4", got.Mouse.Col)
	}
	if got.Mouse.Row != 2 { // 3-1=2
		t.Errorf("row: got %d, want 2", got.Mouse.Row)
	}
}

// ---- Multiple keys in sequence ---------------------------------------------

func TestDecoder_MultipleKeys(t *testing.T) {
	// "ab" → two keys
	got := decode(t, []byte("ab"))
	if len(got) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(got))
	}
	assertKey(t, got[0], keys.Key{Code: keys.KeyCode('a')})
	assertKey(t, got[1], keys.Key{Code: keys.KeyCode('b')})
}

func TestDecoder_EscapeFollowedByLetter_ThenMore(t *testing.T) {
	// ESC 'a' 'b' → M-a, b
	got := decode(t, []byte{0x1B, 'a', 'b'})
	if len(got) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(got))
	}
	assertKey(t, got[0], mustParse(t, "M-a"))
	assertKey(t, got[1], keys.Key{Code: keys.KeyCode('b')})
}

// ---- Table / Registry -------------------------------------------------------

func TestTable_BindLookup(t *testing.T) {
	tbl := keys.NewTable()
	k, _ := keys.Parse("C-b")
	tbl.Bind(k, "prefix")

	cmd, ok := tbl.Lookup(k)
	if !ok {
		t.Fatal("Lookup: expected binding, got none")
	}
	if cmd != "prefix" {
		t.Errorf("Lookup: got %v, want \"prefix\"", cmd)
	}
}

func TestTable_Unbind(t *testing.T) {
	tbl := keys.NewTable()
	k, _ := keys.Parse("C-b")
	tbl.Bind(k, "prefix")
	tbl.Unbind(k)

	_, ok := tbl.Lookup(k)
	if ok {
		t.Error("after Unbind, Lookup should return false")
	}
}

func TestTable_LookupMiss(t *testing.T) {
	tbl := keys.NewTable()
	k, _ := keys.Parse("C-a")
	_, ok := tbl.Lookup(k)
	if ok {
		t.Error("Lookup on empty table should return false")
	}
}

func TestTable_Len(t *testing.T) {
	tbl := keys.NewTable()
	if tbl.Len() != 0 {
		t.Errorf("Len of empty table: got %d", tbl.Len())
	}
	k, _ := keys.Parse("C-b")
	tbl.Bind(k, "x")
	if tbl.Len() != 1 {
		t.Errorf("Len after one bind: got %d", tbl.Len())
	}
}

func TestRegistry_RegisterGet(t *testing.T) {
	reg := keys.NewRegistry()
	tbl := keys.NewTable()
	reg.Register("root", tbl)

	got, ok := reg.Get("root")
	if !ok {
		t.Fatal("Get: expected table, got none")
	}
	if got != tbl {
		t.Error("Get: returned wrong table pointer")
	}
}

func TestRegistry_GetMissing(t *testing.T) {
	reg := keys.NewRegistry()
	_, ok := reg.Get("nonexistent")
	if ok {
		t.Error("Get on empty registry should return false")
	}
}

func TestRegistry_Remove(t *testing.T) {
	reg := keys.NewRegistry()
	reg.Register("root", keys.NewTable())
	reg.Remove("root")
	_, ok := reg.Get("root")
	if ok {
		t.Error("after Remove, Get should return false")
	}
}

func TestRegistry_Names(t *testing.T) {
	reg := keys.NewRegistry()
	reg.Register("root", keys.NewTable())
	reg.Register("prefix", keys.NewTable())
	names := reg.Names()
	if len(names) != 2 {
		t.Errorf("Names: got %v, want 2 entries", names)
	}
}

func TestTable_BindingResolution(t *testing.T) {
	// Simulate: registry with "root" table, look up C-b → "prefix"
	reg := keys.NewRegistry()
	root := keys.NewTable()
	prefix := keys.NewTable()
	reg.Register("root", root)
	reg.Register("prefix", prefix)

	kb, _ := keys.Parse("C-b")
	root.Bind(kb, "switch-table prefix")

	ksplit, _ := keys.Parse("\"")
	prefix.Bind(ksplit, "split-window -h")

	tbl, _ := reg.Get("root")
	cmd, ok := tbl.Lookup(kb)
	if !ok || cmd != "switch-table prefix" {
		t.Errorf("root C-b: got %v %v", cmd, ok)
	}

	tbl2, _ := reg.Get("prefix")
	cmd2, ok2 := tbl2.Lookup(ksplit)
	if !ok2 || cmd2 != "split-window -h" {
		t.Errorf("prefix \": got %v %v", cmd2, ok2)
	}
}

// ---- helpers ---------------------------------------------------------------

func mustParse(t *testing.T, s string) keys.Key {
	t.Helper()
	k, err := keys.Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return k
}
