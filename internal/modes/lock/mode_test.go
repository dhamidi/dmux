package lock

import (
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
)

func newLock(verify func(string) bool) *Mode {
	return New(modes.Rect{X: 0, Y: 0, Width: 80, Height: 24}, verify)
}

// TestLock_CaptureFocus verifies the overlay always captures keyboard focus.
func TestLock_CaptureFocus(t *testing.T) {
	ov := newLock(func(string) bool { return false })
	if !ov.CaptureFocus() {
		t.Error("CaptureFocus() = false, want true")
	}
}

// TestLock_CorrectPassphrase verifies that entering the correct passphrase
// returns a CloseMode outcome.
func TestLock_CorrectPassphrase(t *testing.T) {
	secret := "s3cr3t"
	ov := newLock(func(p string) bool { return p == secret })

	for _, r := range secret {
		outcome := ov.Key(keys.Key{Code: keys.KeyCode(r)})
		if outcome.Kind != modes.KindConsumed {
			t.Errorf("Key(%q): got %v, want Consumed", string(r), outcome.Kind)
		}
	}

	outcome := ov.Key(keys.Key{Code: keys.CodeEnter})
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("Enter with correct passphrase: got %v, want CloseMode", outcome.Kind)
	}
}

// TestLock_IncorrectPassphrase verifies that a wrong passphrase keeps the
// overlay open, clears the buffer, and sets a non-empty error message.
func TestLock_IncorrectPassphrase(t *testing.T) {
	ov := newLock(func(p string) bool { return p == "correct" })

	for _, r := range "wrong" {
		ov.Key(keys.Key{Code: keys.KeyCode(r)})
	}

	outcome := ov.Key(keys.Key{Code: keys.CodeEnter})
	if outcome.Kind == modes.KindCloseMode {
		t.Error("Enter with wrong passphrase should not close the overlay")
	}
	if ov.Buf() != "" {
		t.Errorf("buffer after wrong attempt = %q, want empty", ov.Buf())
	}
	if ov.ErrMsg() == "" {
		t.Error("ErrMsg() is empty after wrong passphrase, want non-empty error")
	}
}

// TestLock_AllKeysBlocked verifies that no key event produces a Passthrough
// outcome while the screen is locked.
func TestLock_AllKeysBlocked(t *testing.T) {
	ov := newLock(func(string) bool { return false })

	testKeys := []keys.Key{
		{Code: keys.KeyCode('a')},
		{Code: keys.KeyCode('z')},
		{Code: keys.KeyCode('0')},
		{Code: keys.CodeEnter},
		{Code: keys.CodeEscape},
		{Code: keys.CodeBackspace},
		{Code: keys.KeyCode(' ')},
	}

	for _, k := range testKeys {
		outcome := ov.Key(k)
		if outcome.Kind == modes.KindPassthrough {
			t.Errorf("Key(%v) passed through – expected Consumed or CloseMode", k)
		}
	}
}

// TestLock_EscapeClearsBuffer verifies Escape clears the buffer without
// closing the overlay.
func TestLock_EscapeClearsBuffer(t *testing.T) {
	ov := newLock(func(string) bool { return false })

	for _, r := range "partial" {
		ov.Key(keys.Key{Code: keys.KeyCode(r)})
	}
	if ov.Buf() == "" {
		t.Fatal("buffer should be non-empty before Escape")
	}

	outcome := ov.Key(keys.Key{Code: keys.CodeEscape})
	if outcome.Kind == modes.KindCloseMode {
		t.Error("Escape must not close the lock overlay")
	}
	if ov.Buf() != "" {
		t.Errorf("buffer after Escape = %q, want empty", ov.Buf())
	}
}

// TestLock_BackspaceEditsBuffer verifies Backspace removes the last typed
// character.
func TestLock_BackspaceEditsBuffer(t *testing.T) {
	ov := newLock(func(string) bool { return false })

	for _, r := range "abc" {
		ov.Key(keys.Key{Code: keys.KeyCode(r)})
	}
	ov.Key(keys.Key{Code: keys.CodeBackspace})

	if got := ov.Buf(); got != "ab" {
		t.Errorf("buffer after Backspace = %q, want %q", got, "ab")
	}
}

// TestLock_MouseConsumed verifies mouse events do not pass through.
func TestLock_MouseConsumed(t *testing.T) {
	ov := newLock(func(string) bool { return false })
	outcome := ov.Mouse(keys.MouseEvent{})
	if outcome.Kind == modes.KindPassthrough {
		t.Error("Mouse event passed through – expected Consumed")
	}
}

// TestLock_RenderDoesNotPanic exercises Render for various screen sizes to
// ensure it does not panic.
func TestLock_RenderDoesNotPanic(t *testing.T) {
	cases := [][2]int{{80, 24}, {1, 1}, {0, 0}, {40, 10}, {200, 50}}
	for _, c := range cases {
		w, h := c[0], c[1]
		ov := New(modes.Rect{Width: w, Height: h}, func(string) bool { return false })
		dst := make([]modes.Cell, w*h)
		ov.Render(dst) // must not panic
	}
}

// TestLock_RenderShowsStars verifies that the render output contains a '*'
// for each character typed into the password buffer.
func TestLock_RenderShowsStars(t *testing.T) {
	ov := newLock(func(string) bool { return false })
	for _, r := range "abc" {
		ov.Key(keys.Key{Code: keys.KeyCode(r)})
	}

	dst := make([]modes.Cell, 80*24)
	ov.Render(dst)

	stars := 0
	for _, cell := range dst {
		if cell.Char == '*' {
			stars++
		}
	}
	if stars != 3 {
		t.Errorf("Render: found %d '*' cells, want 3 (one per typed char)", stars)
	}
}

// TestLock_ErrorClearedOnCorrectEntry verifies that a previous error message
// is cleared when the correct passphrase is entered.
func TestLock_ErrorClearedOnCorrectEntry(t *testing.T) {
	callCount := 0
	ov := newLock(func(p string) bool {
		callCount++
		return callCount >= 2 // first call fails, second succeeds
	})

	// First attempt – should fail and set error.
	for _, r := range "wrong" {
		ov.Key(keys.Key{Code: keys.KeyCode(r)})
	}
	ov.Key(keys.Key{Code: keys.CodeEnter})
	if ov.ErrMsg() == "" {
		t.Error("ErrMsg should be set after failed attempt")
	}

	// Second attempt – should succeed.
	for _, r := range "right" {
		ov.Key(keys.Key{Code: keys.KeyCode(r)})
	}
	outcome := ov.Key(keys.Key{Code: keys.CodeEnter})
	if outcome.Kind != modes.KindCloseMode {
		t.Error("Second attempt should close the overlay")
	}
}
