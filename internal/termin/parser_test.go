package termin

import (
	"bytes"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/termcaps"
)

// asKeyEvent unwraps the single emission produced by feed b as a
// KeyEvent. It fails the test if the parser produced any other
// number of emissions or a non-KeyEvent.
func asKeyEvent(t *testing.T, p *Parser, b []byte) KeyEvent {
	t.Helper()
	ems := p.Feed(b)
	if len(ems) != 1 {
		t.Fatalf("Feed(% x) produced %d emissions, want 1: %#v", b, len(ems), ems)
	}
	ke, ok := ems[0].Event.(KeyEvent)
	if !ok {
		t.Fatalf("Feed(% x)[0] is %T, want KeyEvent", b, ems[0].Event)
	}
	return ke
}

// asEmission returns the single emission produced by feed b. It
// fails the test if the parser produced any other number of
// emissions.
func asEmission(t *testing.T, p *Parser, b []byte) Emission {
	t.Helper()
	ems := p.Feed(b)
	if len(ems) != 1 {
		t.Fatalf("Feed(% x) produced %d emissions, want 1: %#v", b, len(ems), ems)
	}
	return ems[0]
}

// Printable lowercase letter "a": Key=KeyA, Mods=0, Text="a".
func TestFeedLowercaseLetter(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	got := asKeyEvent(t, p, []byte{'a'})
	want := keys.Event{Action: keys.ActionPress, Key: keys.KeyA, Text: "a"}
	if got.Event != want {
		t.Errorf("got %+v, want %+v", got.Event, want)
	}
}

// Printable uppercase letter "A" keeps Shift on the Event; the
// keys.Code normalizer folds it away for bindings, so we verify
// that path too.
func TestFeedUppercaseLetterHasShift(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	got := asKeyEvent(t, p, []byte{'A'})
	want := keys.Event{Action: keys.ActionPress, Key: keys.KeyA, Mods: keys.ModShift, Text: "A"}
	if got.Event != want {
		t.Errorf("got %+v, want %+v", got.Event, want)
	}
	code, ok := keys.Code(got.Event)
	if !ok || code != (keys.KeyCode{Key: keys.KeyA}) {
		t.Errorf("Code(%+v) = %+v, ok=%v; want KeyCode{KeyA}", got.Event, code, ok)
	}
}

// C-b (byte 0x02) is the tmux prefix. This test gates the whole
// walking-skeleton effort.
func TestFeedCtrlB(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	got := asKeyEvent(t, p, []byte{0x02})
	want := keys.Event{Action: keys.ActionPress, Key: keys.KeyB, Mods: keys.ModCtrl}
	if got.Event != want {
		t.Errorf("got %+v, want %+v", got.Event, want)
	}
}

// Ctrl-Space arrives as NUL (0x00).
func TestFeedCtrlSpace(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	got := asKeyEvent(t, p, []byte{0x00})
	want := keys.Event{Action: keys.ActionPress, Key: keys.KeySpace, Mods: keys.ModCtrl}
	if got.Event != want {
		t.Errorf("got %+v, want %+v", got.Event, want)
	}
}

// Enter (CR) maps to KeyEnter with no mods.
func TestFeedEnter(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	got := asKeyEvent(t, p, []byte{0x0D})
	want := keys.Event{Action: keys.ActionPress, Key: keys.KeyEnter}
	if got.Event != want {
		t.Errorf("got %+v, want %+v", got.Event, want)
	}
}

// LF is also treated as Enter.
func TestFeedLFIsEnter(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	got := asKeyEvent(t, p, []byte{0x0A})
	want := keys.Event{Action: keys.ActionPress, Key: keys.KeyEnter}
	if got.Event != want {
		t.Errorf("got %+v, want %+v", got.Event, want)
	}
}

// Tab.
func TestFeedTab(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	got := asKeyEvent(t, p, []byte{0x09})
	want := keys.Event{Action: keys.ActionPress, Key: keys.KeyTab}
	if got.Event != want {
		t.Errorf("got %+v, want %+v", got.Event, want)
	}
}

// DEL (0x7F) is the common Backspace on Unix terminals.
func TestFeedBackspaceDEL(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	got := asKeyEvent(t, p, []byte{0x7F})
	want := keys.Event{Action: keys.ActionPress, Key: keys.KeyBackspace}
	if got.Event != want {
		t.Errorf("got %+v, want %+v", got.Event, want)
	}
}

// BS (0x08) also maps to Backspace.
func TestFeedBackspaceBS(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	got := asKeyEvent(t, p, []byte{0x08})
	want := keys.Event{Action: keys.ActionPress, Key: keys.KeyBackspace}
	if got.Event != want {
		t.Errorf("got %+v, want %+v", got.Event, want)
	}
}

// A bare ESC does not emit an event until the escape-timeout
// deadline passes. Deadline() reports the pending time between
// Feed and Tick, and reports nothing before Feed or after Tick.
func TestFeedBareEscape(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	if _, ok := p.Deadline(); ok {
		t.Fatal("pre-Feed Deadline reported pending timer")
	}
	evs := p.Feed([]byte{0x1B})
	if len(evs) != 0 {
		t.Fatalf("Feed(ESC) emitted %d events, want 0: %#v", len(evs), evs)
	}
	dl, ok := p.Deadline()
	if !ok {
		t.Fatal("post-Feed Deadline missing")
	}
	// Tick before the deadline emits nothing.
	if evs := p.Tick(dl.Add(-time.Millisecond)); len(evs) != 0 {
		t.Fatalf("Tick before deadline emitted %d events, want 0: %#v", len(evs), evs)
	}
	if _, ok := p.Deadline(); !ok {
		t.Fatal("Tick before deadline cleared the pending timer")
	}
	// Tick at or after the deadline emits exactly one Escape.
	evs = p.Tick(dl.Add(time.Millisecond))
	if len(evs) != 1 {
		t.Fatalf("Tick after deadline emitted %d events, want 1: %#v", len(evs), evs)
	}
	ke, ok := evs[0].Event.(KeyEvent)
	if !ok || ke.Key != keys.KeyEscape {
		t.Errorf("Tick event = %#v, want KeyEscape", evs[0])
	}
	if evs[0].Bytes != nil {
		t.Errorf("Tick-emitted KeyEscape Bytes = %q, want nil", evs[0].Bytes)
	}
	if _, ok := p.Deadline(); ok {
		t.Error("post-Tick Deadline still pending")
	}
}

// ESC [ A arriving in one feed emits Up with no stray Escape.
func TestFeedArrowUpSingleFeed(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	got := asKeyEvent(t, p, []byte{0x1B, '[', 'A'})
	want := keys.Event{Action: keys.ActionPress, Key: keys.KeyArrowUp}
	if got.Event != want {
		t.Errorf("got %+v, want %+v", got.Event, want)
	}
	if _, ok := p.Deadline(); ok {
		t.Error("Deadline still pending after full sequence")
	}
}

// ESC [ A split across three Feed calls still composes correctly.
func TestFeedArrowUpSplit(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	if evs := p.Feed([]byte{0x1B}); len(evs) != 0 {
		t.Fatalf("first chunk emitted %d events", len(evs))
	}
	if evs := p.Feed([]byte{'['}); len(evs) != 0 {
		t.Fatalf("second chunk emitted %d events", len(evs))
	}
	evs := p.Feed([]byte{'A'})
	if len(evs) != 1 {
		t.Fatalf("final chunk emitted %d events: %#v", len(evs), evs)
	}
	ke := evs[0].Event.(KeyEvent)
	if ke.Key != keys.KeyArrowUp {
		t.Errorf("got key %s, want ArrowUp", ke.Key)
	}
	// Cross-call byte accounting: the final emission carries the
	// whole envelope spanning all three Feed calls, not just the
	// last byte.
	want := []byte{0x1B, '[', 'A'}
	if !bytes.Equal(evs[0].Bytes, want) {
		t.Errorf("Bytes = % x, want % x", evs[0].Bytes, want)
	}
}

// ESC a → Alt-a (ESC+printable short-circuit).
func TestFeedAltA(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	got := asKeyEvent(t, p, []byte{0x1B, 'a'})
	want := keys.Event{Action: keys.ActionPress, Key: keys.KeyA, Mods: keys.ModAlt, Text: "a"}
	if got.Event != want {
		t.Errorf("got %+v, want %+v", got.Event, want)
	}
}

// ESC N → Alt-Shift-n: shift bit persists through the Alt merge.
func TestFeedAltShiftN(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	got := asKeyEvent(t, p, []byte{0x1B, 'N'})
	want := keys.Event{Action: keys.ActionPress, Key: keys.KeyN, Mods: keys.ModAlt | keys.ModShift, Text: "N"}
	if got.Event != want {
		t.Errorf("got %+v, want %+v", got.Event, want)
	}
}

// CSI 11 ~ is F1.
func TestFeedF1Tilde(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	got := asKeyEvent(t, p, []byte{0x1B, '[', '1', '1', '~'})
	want := keys.Event{Action: keys.ActionPress, Key: keys.KeyF1}
	if got.Event != want {
		t.Errorf("got %+v, want %+v", got.Event, want)
	}
}

// SS3 variant of F1: ESC O P.
func TestFeedF1SS3(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	got := asKeyEvent(t, p, []byte{0x1B, 'O', 'P'})
	if got.Key != keys.KeyF1 {
		t.Errorf("got %s, want F1", got.Key)
	}
}

// CSI H with no params is Home.
func TestFeedHomeLetterH(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	got := asKeyEvent(t, p, []byte{0x1B, '[', 'H'})
	if got.Key != keys.KeyHome {
		t.Errorf("got %s, want Home", got.Key)
	}
}

// CSI 1 ; 2 C → Shift-Right.
func TestFeedShiftRightModified(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	got := asKeyEvent(t, p, []byte{0x1B, '[', '1', ';', '2', 'C'})
	want := keys.Event{Action: keys.ActionPress, Key: keys.KeyArrowRight, Mods: keys.ModShift}
	if got.Event != want {
		t.Errorf("got %+v, want %+v", got.Event, want)
	}
}

// Reset after a partial ESC clears state: the next Feed must
// parse fresh bytes without a lingering ESC-prefix interpretation.
func TestResetClearsPendingEsc(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	p.Feed([]byte{0x1B})
	if _, ok := p.Deadline(); !ok {
		t.Fatal("Feed(ESC) did not set deadline")
	}
	p.Reset()
	if _, ok := p.Deadline(); ok {
		t.Error("Reset did not clear deadline")
	}
	got := asKeyEvent(t, p, []byte{'a'})
	if got.Key != keys.KeyA || got.Mods != 0 {
		t.Errorf("got %+v, want plain a", got.Event)
	}
}

// UTF-8 multibyte "é" (0xC3 0xA9).
func TestFeedUTF8Multibyte(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	evs := p.Feed([]byte{0xC3, 0xA9})
	if len(evs) != 1 {
		t.Fatalf("got %d events, want 1: %#v", len(evs), evs)
	}
	ke := evs[0].Event.(KeyEvent)
	if ke.Text != "é" {
		t.Errorf("Text = %q, want %q", ke.Text, "é")
	}
	if ke.Unshifted != 'é' {
		t.Errorf("Unshifted = %d, want %d", ke.Unshifted, 'é')
	}
	if ke.Key != keys.KeyUnidentified {
		t.Errorf("Key = %s, want Unidentified", ke.Key)
	}
}

// Bracketed paste: CSI 200~ ... CSI 201~ produces a PasteEvent
// with exactly the payload bytes and no stray key events.
func TestFeedBracketedPaste(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	input := []byte{0x1B, '[', '2', '0', '0', '~'}
	input = append(input, []byte("hello")...)
	input = append(input, 0x1B, '[', '2', '0', '1', '~')
	evs := p.Feed(input)
	if len(evs) != 1 {
		t.Fatalf("got %d events, want 1: %#v", len(evs), evs)
	}
	pe, ok := evs[0].Event.(PasteEvent)
	if !ok {
		t.Fatalf("event is %T, want PasteEvent", evs[0].Event)
	}
	if string(pe.Data) != "hello" {
		t.Errorf("Data = %q, want %q", pe.Data, "hello")
	}
	// Paste emission bytes cover the whole envelope — opening
	// marker, payload, closing marker — so passthrough routing
	// forwards the entire paste transparently.
	wantBytes := append([]byte{0x1B, '[', '2', '0', '0', '~'}, []byte("hello")...)
	wantBytes = append(wantBytes, 0x1B, '[', '2', '0', '1', '~')
	if !bytes.Equal(evs[0].Bytes, wantBytes) {
		t.Errorf("Bytes = % x, want % x", evs[0].Bytes, wantBytes)
	}
}

// SGR mouse envelope: CSI < 0 ; 10 ; 20 M emits one MouseEvent.
// Field values are placeholders in M1; we just confirm the bytes
// do not leak through as stray keys.
func TestFeedSGRMouseEnvelope(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	evs := p.Feed([]byte{0x1B, '[', '<', '0', ';', '1', '0', ';', '2', '0', 'M'})
	if len(evs) != 1 {
		t.Fatalf("got %d events, want 1: %#v", len(evs), evs)
	}
	if _, ok := evs[0].Event.(MouseEvent); !ok {
		t.Errorf("event is %T, want MouseEvent", evs[0].Event)
	}
	// Mouse emission bytes cover the full CSI-< envelope.
	want := []byte{0x1B, '[', '<', '0', ';', '1', '0', ';', '2', '0', 'M'}
	if !bytes.Equal(evs[0].Bytes, want) {
		t.Errorf("Bytes = % x, want % x", evs[0].Bytes, want)
	}
}

// Focus in / out: CSI I and CSI O (the latter only matches
// through the CSI path, not SS3, because CSI has at least one
// '[' prefix byte before the dispatch).
func TestFeedFocusIn(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	evs := p.Feed([]byte{0x1B, '[', 'I'})
	if len(evs) != 1 {
		t.Fatalf("got %d events, want 1", len(evs))
	}
	fe, ok := evs[0].Event.(FocusEvent)
	if !ok || !fe.In {
		t.Errorf("event = %#v, want FocusEvent{In:true}", evs[0].Event)
	}
}

// An unrecognized CSI sequence is silently dropped; no stray
// keys must leak through.
func TestFeedUnknownCSISilent(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	evs := p.Feed([]byte{0x1B, '[', '9', '9', 'z'})
	if len(evs) != 0 {
		t.Errorf("unknown CSI emitted %d events: %#v", len(evs), evs)
	}
}

// ESC then a second ESC with no body: the first becomes a
// stand-alone Escape, the second stays pending.
func TestFeedDoubleEscape(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	evs := p.Feed([]byte{0x1B, 0x1B})
	if len(evs) != 1 {
		t.Fatalf("got %d events, want 1: %#v", len(evs), evs)
	}
	ke := evs[0].Event.(KeyEvent)
	if ke.Key != keys.KeyEscape {
		t.Errorf("got %s, want Escape", ke.Key)
	}
	// The first ESC is the emitted event — its raw bytes are one
	// ESC. The second ESC remains pending and has not emitted yet.
	if !bytes.Equal(evs[0].Bytes, []byte{0x1B}) {
		t.Errorf("Bytes = % x, want 1B", evs[0].Bytes)
	}
	if _, ok := p.Deadline(); !ok {
		t.Error("second ESC should leave a pending deadline")
	}
}

// Ctrl-] (0x1D) maps to ]+ModCtrl.
func TestFeedCtrlBracketRight(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	got := asKeyEvent(t, p, []byte{0x1D})
	want := keys.Event{Action: keys.ActionPress, Key: keys.KeyBracketRight, Mods: keys.ModCtrl}
	if got.Event != want {
		t.Errorf("got %+v, want %+v", got.Event, want)
	}
}

// The Ctrl-b binding lookup works against a freshly parsed
// 0x02 byte. Exercised here to catch integration regressions
// between termin and keys.Code early — this is the whole point
// of the M1 walking skeleton.
func TestCtrlBLookupIntegration(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	ke := asKeyEvent(t, p, []byte{0x02})
	code, ok := keys.Code(ke.Event)
	if !ok {
		t.Fatal("keys.Code rejected Ctrl-b")
	}
	want := keys.KeyCode{Key: keys.KeyB, Mods: keys.ModCtrl}
	if code != want {
		t.Errorf("got %+v, want %+v", code, want)
	}
}

// KKP CSI u is only decoded for KKP-capable profiles. With
// Ghostty (KKP=true), "CSI 97 u" becomes 'a'.
func TestFeedKKPLowercaseA(t *testing.T) {
	p := NewParser(termcaps.Ghostty)
	got := asKeyEvent(t, p, []byte{0x1B, '[', '9', '7', 'u'})
	if got.Key != keys.KeyA || got.Text != "a" {
		t.Errorf("got %+v, want plain a", got.Event)
	}
}

// Without KKP the same bytes are silently dropped — the legacy
// profile has no defined meaning for CSI u and would otherwise
// misreport.
func TestFeedKKPIgnoredWithoutFeature(t *testing.T) {
	p := NewParser(termcaps.XTermJSLegacy)
	evs := p.Feed([]byte{0x1B, '[', '9', '7', 'u'})
	if len(evs) != 0 {
		t.Errorf("legacy profile emitted %d events for CSI u, want 0", len(evs))
	}
}

// KKP event type 3 = release.
func TestFeedKKPRelease(t *testing.T) {
	p := NewParser(termcaps.Ghostty)
	got := asKeyEvent(t, p, []byte{0x1B, '[', '9', '7', ';', '1', ':', '3', 'u'})
	if got.Action != keys.ActionRelease {
		t.Errorf("Action = %s, want release", got.Action)
	}
}

// Multiple bytes in one Feed: "abc" produces three events in order.
func TestFeedMultipleLetters(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	evs := p.Feed([]byte("abc"))
	if len(evs) != 3 {
		t.Fatalf("got %d events, want 3: %#v", len(evs), evs)
	}
	wantKeys := []keys.Key{keys.KeyA, keys.KeyB, keys.KeyC}
	for i, em := range evs {
		ke := em.Event.(KeyEvent)
		if ke.Key != wantKeys[i] {
			t.Errorf("event %d key = %s, want %s", i, ke.Key, wantKeys[i])
		}
	}
}

// A bare ESC followed later by a non-prefix byte (e.g. 'a') still
// parses as Alt-a: the ESC+printable shortcut is timing-insensitive
// as long as Tick has not yet fired. This models a slow two-feed
// delivery on a laggy link.
func TestFeedSlowAltA(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	p.Feed([]byte{0x1B})
	evs := p.Feed([]byte{'a'})
	if len(evs) != 1 {
		t.Fatalf("got %d events, want 1", len(evs))
	}
	ke := evs[0].Event.(KeyEvent)
	want := keys.Event{Action: keys.ActionPress, Key: keys.KeyA, Mods: keys.ModAlt, Text: "a"}
	if ke.Event != want {
		t.Errorf("got %+v, want %+v", ke.Event, want)
	}
	// Cross-call byte accounting: the Alt-a emission carries ESC+a,
	// not just the second Feed's 'a'.
	if !bytes.Equal(evs[0].Bytes, []byte{0x1B, 'a'}) {
		t.Errorf("Bytes = % x, want 1b 61", evs[0].Bytes)
	}
}

// Every emission carries the raw input bytes that produced it so
// the server's routing layer can decide to drop (bound command
// fired) or forward (unbound key: passthrough to pty). These cases
// sweep across the dispatch shapes to pin the contract down.
func TestEmissionBytesCoverage(t *testing.T) {
	cases := []struct {
		name    string
		profile termcaps.Profile
		in      []byte
		want    []byte
	}{
		{"printable lowercase", termcaps.Unknown, []byte{'a'}, []byte{'a'}},
		{"printable uppercase", termcaps.Unknown, []byte{'A'}, []byte{'A'}},
		{"c0 control ctrl-b", termcaps.Unknown, []byte{0x02}, []byte{0x02}},
		{"c0 enter", termcaps.Unknown, []byte{0x0D}, []byte{0x0D}},
		{"csi arrow up", termcaps.Unknown, []byte{0x1B, '[', 'A'}, []byte{0x1B, '[', 'A'}},
		{"csi home H", termcaps.Unknown, []byte{0x1B, '[', 'H'}, []byte{0x1B, '[', 'H'}},
		{"csi f1 tilde", termcaps.Unknown, []byte{0x1B, '[', '1', '1', '~'}, []byte{0x1B, '[', '1', '1', '~'}},
		{"csi shift-right", termcaps.Unknown, []byte{0x1B, '[', '1', ';', '2', 'C'}, []byte{0x1B, '[', '1', ';', '2', 'C'}},
		{"ss3 f1", termcaps.Unknown, []byte{0x1B, 'O', 'P'}, []byte{0x1B, 'O', 'P'}},
		{"alt-a envelope", termcaps.Unknown, []byte{0x1B, 'a'}, []byte{0x1B, 'a'}},
		{"utf8 é two-byte", termcaps.Unknown, []byte{0xC3, 0xA9}, []byte{0xC3, 0xA9}},
		{"focus in", termcaps.Unknown, []byte{0x1B, '[', 'I'}, []byte{0x1B, '[', 'I'}},
		{"kkp lowercase a", termcaps.Ghostty, []byte{0x1B, '[', '9', '7', 'u'}, []byte{0x1B, '[', '9', '7', 'u'}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewParser(tc.profile)
			em := asEmission(t, p, tc.in)
			if !bytes.Equal(em.Bytes, tc.want) {
				t.Errorf("Bytes = % x, want % x", em.Bytes, tc.want)
			}
		})
	}
}

// Bytes must be a fresh allocation — Feed must tolerate callers
// reusing the input buffer immediately after the call returns.
// This test mutates the input slice after Feed and checks that the
// emission's Bytes are unaffected.
func TestEmissionBytesIndependentOfInput(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	in := []byte{0x1B, '[', 'A'}
	ems := p.Feed(in)
	if len(ems) != 1 {
		t.Fatalf("got %d emissions, want 1", len(ems))
	}
	// Overwrite the input buffer.
	for i := range in {
		in[i] = 0xFF
	}
	want := []byte{0x1B, '[', 'A'}
	if !bytes.Equal(ems[0].Bytes, want) {
		t.Errorf("Bytes = % x, want % x (input mutation leaked)", ems[0].Bytes, want)
	}
}

// Tick-emitted KeyEscape has Bytes == nil: the ESC byte was
// consumed by an earlier Feed, and the emission is time-driven
// rather than byte-driven. Callers treat nil Bytes as "nothing to
// forward" in the passthrough path.
func TestTickEmittedEscapeHasNilBytes(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	if ems := p.Feed([]byte{0x1B}); len(ems) != 0 {
		t.Fatalf("Feed(ESC) emitted %d emissions, want 0", len(ems))
	}
	dl, ok := p.Deadline()
	if !ok {
		t.Fatal("Deadline missing after Feed(ESC)")
	}
	ems := p.Tick(dl.Add(time.Millisecond))
	if len(ems) != 1 {
		t.Fatalf("Tick emitted %d emissions, want 1", len(ems))
	}
	if ems[0].Bytes != nil {
		t.Errorf("Tick-emitted KeyEscape Bytes = % x, want nil", ems[0].Bytes)
	}
	ke, ok := ems[0].Event.(KeyEvent)
	if !ok || ke.Key != keys.KeyEscape {
		t.Errorf("event = %#v, want KeyEscape", ems[0].Event)
	}
}

// Across two Feed calls the final emission carries the whole
// envelope: feed "\x1B[" then "A", assert the second Feed's
// emission has Bytes == "\x1B[A". Documents the cross-call byte
// accounting contract that routing code relies on.
func TestCrossCallCSIEnvelope(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	if ems := p.Feed([]byte{0x1B, '['}); len(ems) != 0 {
		t.Fatalf("first Feed emitted %d, want 0", len(ems))
	}
	ems := p.Feed([]byte{'A'})
	if len(ems) != 1 {
		t.Fatalf("second Feed emitted %d, want 1", len(ems))
	}
	want := []byte{0x1B, '[', 'A'}
	if !bytes.Equal(ems[0].Bytes, want) {
		t.Errorf("Bytes = % x, want % x", ems[0].Bytes, want)
	}
}

// An Alt-a emission straddling two Feed calls still carries the
// full ESC+a envelope. The ESC arrives on the first Feed (buffered
// as pending); the 'a' arrives on the second Feed and resolves as
// Alt-a. The emission's Bytes must be both.
func TestCrossCallAltPrefix(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	if ems := p.Feed([]byte{0x1B}); len(ems) != 0 {
		t.Fatalf("first Feed emitted %d, want 0", len(ems))
	}
	ems := p.Feed([]byte{'a'})
	if len(ems) != 1 {
		t.Fatalf("second Feed emitted %d, want 1", len(ems))
	}
	want := []byte{0x1B, 'a'}
	if !bytes.Equal(ems[0].Bytes, want) {
		t.Errorf("Bytes = % x, want % x", ems[0].Bytes, want)
	}
}

// A bracketed paste split across three Feed calls still emits one
// paste emission whose Bytes cover the whole envelope. The
// opening marker arrives on the first Feed, the payload on the
// second, and the closing marker on the third; only the third
// Feed produces an emission, and that emission carries the full
// opening+payload+closing bytes.
func TestCrossCallBracketedPaste(t *testing.T) {
	p := NewParser(termcaps.Unknown)
	if ems := p.Feed([]byte{0x1B, '[', '2', '0', '0', '~'}); len(ems) != 0 {
		t.Fatalf("first Feed emitted %d, want 0", len(ems))
	}
	if ems := p.Feed([]byte("hello")); len(ems) != 0 {
		t.Fatalf("second Feed emitted %d, want 0", len(ems))
	}
	ems := p.Feed([]byte{0x1B, '[', '2', '0', '1', '~'})
	if len(ems) != 1 {
		t.Fatalf("third Feed emitted %d, want 1", len(ems))
	}
	pe, ok := ems[0].Event.(PasteEvent)
	if !ok {
		t.Fatalf("event is %T, want PasteEvent", ems[0].Event)
	}
	if string(pe.Data) != "hello" {
		t.Errorf("Data = %q, want %q", pe.Data, "hello")
	}
	want := append([]byte{0x1B, '[', '2', '0', '0', '~'}, []byte("hello")...)
	want = append(want, 0x1B, '[', '2', '0', '1', '~')
	if !bytes.Equal(ems[0].Bytes, want) {
		t.Errorf("Bytes = % x, want % x", ems[0].Bytes, want)
	}
}
