package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	lockmode "github.com/dhamidi/dmux/internal/modes/lock"
	"github.com/dhamidi/dmux/internal/session"
)

// newTestMutatorWithLock creates a minimal serverMutator for lock tests.
// It registers client "c1" (24×80) and wires overlay push/pop callbacks.
func newTestMutatorWithLock(verifyFn func(string) bool) (
	m *serverMutator,
	pushed func() modes.ClientOverlay,
	popped func() bool,
) {
	state := session.NewServer()

	c := session.NewClient(session.ClientID("c1"))
	c.Size = session.Size{Rows: 24, Cols: 80}
	state.Clients[c.ID] = c

	var lastOverlay modes.ClientOverlay
	var popCalled bool

	m = &serverMutator{
		state:    state,
		shutdown: func() {},
		pushOverlayFn: func(_ session.ClientID, ov modes.ClientOverlay) {
			lastOverlay = ov
		},
		popOverlayFn: func(_ session.ClientID) {
			popCalled = true
		},
		lockVerifyFn: verifyFn,
	}

	return m,
		func() modes.ClientOverlay { return lastOverlay },
		func() bool { return popCalled }
}

// TestLockClient_PushesOverlay verifies that LockClient pushes a *lock.Mode
// onto the target client's overlay stack.
func TestLockClient_PushesOverlay(t *testing.T) {
	m, pushed, _ := newTestMutatorWithLock(func(string) bool { return false })

	if err := m.LockClient("c1"); err != nil {
		t.Fatalf("LockClient: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("pushOverlayFn not called or overlay is nil")
	}
	if _, ok := ov.(*lockmode.Mode); !ok {
		t.Errorf("pushed overlay type = %T, want *lockmode.Mode", ov)
	}
}

// TestLockClient_OverlayCoversFullScreen verifies the overlay rect matches the
// client dimensions.
func TestLockClient_OverlayCoversFullScreen(t *testing.T) {
	m, pushed, _ := newTestMutatorWithLock(func(string) bool { return false })

	if err := m.LockClient("c1"); err != nil {
		t.Fatalf("LockClient: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("overlay is nil")
	}
	rect := ov.Rect()
	if rect.X != 0 || rect.Y != 0 {
		t.Errorf("rect origin = (%d,%d), want (0,0)", rect.X, rect.Y)
	}
	if rect.Width != 80 || rect.Height != 24 {
		t.Errorf("rect size = %dx%d, want 80x24", rect.Width, rect.Height)
	}
}

// TestLockClient_UnknownClient returns an error for an unknown client ID.
func TestLockClient_UnknownClient(t *testing.T) {
	m, _, _ := newTestMutatorWithLock(func(string) bool { return false })

	if err := m.LockClient("no-such-client"); err == nil {
		t.Error("LockClient with unknown client should return error, got nil")
	}
}

// TestLockServer_LocksAllClients verifies LockServer calls LockClient for
// every attached client.
func TestLockServer_LocksAllClients(t *testing.T) {
	state := session.NewServer()
	c1 := session.NewClient(session.ClientID("c1"))
	c1.Size = session.Size{Rows: 24, Cols: 80}
	c2 := session.NewClient(session.ClientID("c2"))
	c2.Size = session.Size{Rows: 24, Cols: 80}
	state.Clients[c1.ID] = c1
	state.Clients[c2.ID] = c2

	var lockedClients []session.ClientID

	m := &serverMutator{
		state:    state,
		shutdown: func() {},
		pushOverlayFn: func(id session.ClientID, _ modes.ClientOverlay) {
			lockedClients = append(lockedClients, id)
		},
		popOverlayFn: func(_ session.ClientID) {},
		lockVerifyFn: func(string) bool { return false },
	}

	if err := m.LockServer(); err != nil {
		t.Fatalf("LockServer: %v", err)
	}

	if len(lockedClients) != 2 {
		t.Errorf("LockServer locked %d client(s), want 2; locked: %v", len(lockedClients), lockedClients)
	}
}

// TestLockOverlay_CaptureFocus verifies the lock overlay always captures focus.
func TestLockOverlay_CaptureFocus(t *testing.T) {
	ov := lockmode.New(modes.Rect{Width: 80, Height: 24}, func(string) bool { return false })
	if !ov.CaptureFocus() {
		t.Error("CaptureFocus() = false, want true")
	}
}

// TestLockOverlay_CorrectPassphrase verifies that the correct passphrase
// returns CloseMode and the overlay unlocks.
func TestLockOverlay_CorrectPassphrase(t *testing.T) {
	secret := "s3cr3t"
	ov := lockmode.New(modes.Rect{Width: 80, Height: 24}, func(p string) bool {
		return p == secret
	})

	// Type the correct passphrase.
	for _, r := range secret {
		outcome := ov.Key(keys.Key{Code: keys.KeyCode(r)})
		if outcome.Kind != modes.KindConsumed {
			t.Errorf("Key(%q) outcome = %v, want Consumed", r, outcome.Kind)
		}
	}

	// Press Enter – should unlock.
	outcome := ov.Key(keys.Key{Code: keys.CodeEnter})
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("Enter with correct passphrase: outcome = %v, want CloseMode", outcome.Kind)
	}
}

// TestLockOverlay_IncorrectPassphrase verifies that a wrong passphrase does
// not unlock the overlay, clears the buffer, and sets an error message.
func TestLockOverlay_IncorrectPassphrase(t *testing.T) {
	ov := lockmode.New(modes.Rect{Width: 80, Height: 24}, func(p string) bool {
		return p == "correct"
	})

	// Type a wrong passphrase.
	for _, r := range "wrong" {
		ov.Key(keys.Key{Code: keys.KeyCode(r)})
	}

	outcome := ov.Key(keys.Key{Code: keys.CodeEnter})
	if outcome.Kind == modes.KindCloseMode {
		t.Error("Enter with wrong passphrase: overlay should stay open")
	}
	if ov.Buf() != "" {
		t.Errorf("buffer after wrong attempt = %q, want empty", ov.Buf())
	}
	if ov.ErrMsg() == "" {
		t.Error("ErrMsg() is empty after wrong passphrase, want a non-empty error")
	}
}

// TestLockOverlay_AllKeysBlocked verifies that key events do not pass through
// the lock overlay (every outcome is Consumed or CloseMode, never Passthrough).
func TestLockOverlay_AllKeysBlocked(t *testing.T) {
	ov := lockmode.New(modes.Rect{Width: 80, Height: 24}, func(string) bool { return false })

	testKeys := []keys.Key{
		{Code: keys.KeyCode('a')},
		{Code: keys.KeyCode('z')},
		{Code: keys.CodeEnter},
		{Code: keys.CodeEscape},
		{Code: keys.CodeBackspace},
		{Code: keys.KeyCode('q')},
		{Code: keys.KeyCode(' ')},
	}

	for _, k := range testKeys {
		outcome := ov.Key(k)
		if outcome.Kind == modes.KindPassthrough {
			t.Errorf("Key(%v) passed through – expected Consumed or CloseMode", k)
		}
	}
}

// TestLockOverlay_EscapeClearsBuffer verifies Escape clears the passphrase
// buffer without unlocking.
func TestLockOverlay_EscapeClearsBuffer(t *testing.T) {
	ov := lockmode.New(modes.Rect{Width: 80, Height: 24}, func(string) bool { return false })

	for _, r := range "partial" {
		ov.Key(keys.Key{Code: keys.KeyCode(r)})
	}
	if ov.Buf() == "" {
		t.Fatal("buffer should be non-empty before Escape")
	}

	outcome := ov.Key(keys.Key{Code: keys.CodeEscape})
	if outcome.Kind == modes.KindCloseMode {
		t.Error("Escape should not close the lock overlay")
	}
	if ov.Buf() != "" {
		t.Errorf("buffer after Escape = %q, want empty", ov.Buf())
	}
}

// TestLockOverlay_BackspaceEditsBuffer verifies Backspace removes the last
// character from the buffer.
func TestLockOverlay_BackspaceEditsBuffer(t *testing.T) {
	ov := lockmode.New(modes.Rect{Width: 80, Height: 24}, func(string) bool { return false })

	for _, r := range "abc" {
		ov.Key(keys.Key{Code: keys.KeyCode(r)})
	}
	ov.Key(keys.Key{Code: keys.CodeBackspace})

	if got := ov.Buf(); got != "ab" {
		t.Errorf("buffer after Backspace = %q, want %q", got, "ab")
	}
}

// TestLockOverlay_MouseConsumed verifies mouse events are blocked.
func TestLockOverlay_MouseConsumed(t *testing.T) {
	ov := lockmode.New(modes.Rect{Width: 80, Height: 24}, func(string) bool { return false })
	outcome := ov.Mouse(keys.MouseEvent{})
	if outcome.Kind == modes.KindPassthrough {
		t.Error("Mouse event passed through – expected Consumed")
	}
}

// TestLockOverlay_RenderDoesNotPanic exercises Render to ensure it does not
// panic on normal and edge-case sizes.
func TestLockOverlay_RenderDoesNotPanic(t *testing.T) {
	sizes := [][2]int{{80, 24}, {1, 1}, {0, 0}, {40, 10}}
	for _, s := range sizes {
		w, h := s[0], s[1]
		ov := lockmode.New(modes.Rect{Width: w, Height: h}, func(string) bool { return false })
		dst := make([]modes.Cell, w*h)
		ov.Render(dst) // must not panic
	}
}
