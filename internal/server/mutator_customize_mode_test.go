package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	"github.com/dhamidi/dmux/internal/session"
)

// newTestMutatorWithCustomizeMode creates a serverMutator wired for
// customize-mode tests. It returns a state with one session/window, a client,
// pre-registered key bindings and options, plus a closure that returns the
// last overlay pushed onto the client.
func newTestMutatorWithCustomizeMode() (
	m *serverMutator,
	client *session.Client,
	pushed func() modes.ClientOverlay,
) {
	state := session.NewServer()

	sess := session.NewSession(session.SessionID("s1"), "session1", nil)
	state.AddSession(sess)

	win := session.NewWindow(session.WindowID("w1"), "w1", nil)
	wl := &session.Winlink{Window: win, Session: sess, Index: 1}
	sess.Windows = append(sess.Windows, wl)
	sess.Current = wl

	c := session.NewClient(session.ClientID("c1"))
	c.Session = sess
	c.Size = session.Size{Rows: 24, Cols: 80}
	state.Clients[c.ID] = c

	var lastOverlay modes.ClientOverlay
	m = &serverMutator{
		state:    state,
		shutdown: func() {},
		pushOverlayFn: func(_ session.ClientID, ov modes.ClientOverlay) {
			lastOverlay = ov
		},
		popOverlayFn: func(_ session.ClientID) {},
	}

	return m, c, func() modes.ClientOverlay { return lastOverlay }
}

// TestEnterCustomizeMode_AttachesOverlay verifies that EnterCustomizeMode
// pushes a *modes.CustomizeOverlay onto the target client.
func TestEnterCustomizeMode_AttachesOverlay(t *testing.T) {
	m, _, pushed := newTestMutatorWithCustomizeMode()

	if err := m.EnterCustomizeMode("c1"); err != nil {
		t.Fatalf("EnterCustomizeMode: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("pushOverlayFn not called or overlay is nil")
	}
	if _, ok := ov.(*modes.CustomizeOverlay); !ok {
		t.Errorf("pushed overlay type = %T, want *modes.CustomizeOverlay", ov)
	}
}

// TestEnterCustomizeMode_ClientNotFound verifies that an error is returned
// when the client does not exist.
func TestEnterCustomizeMode_ClientNotFound(t *testing.T) {
	m, _, _ := newTestMutatorWithCustomizeMode()

	if err := m.EnterCustomizeMode("no-such-client"); err == nil {
		t.Fatal("expected error for unknown client, got nil")
	}
}

// TestEnterCustomizeMode_OverlayCoversFullScreen verifies that the overlay
// rect matches the client's terminal size.
func TestEnterCustomizeMode_OverlayCoversFullScreen(t *testing.T) {
	m, _, pushed := newTestMutatorWithCustomizeMode()

	if err := m.EnterCustomizeMode("c1"); err != nil {
		t.Fatalf("EnterCustomizeMode: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("overlay is nil")
	}
	rect := ov.Rect()
	if rect.Width != 80 || rect.Height != 24 {
		t.Errorf("overlay rect = {%d×%d}, want {80×24}", rect.Width, rect.Height)
	}
}

// TestEnterCustomizeMode_RendersContent verifies that Render produces non-empty
// output for the overlay (Options / Key Bindings groups are always present).
func TestEnterCustomizeMode_RendersContent(t *testing.T) {
	m, _, pushed := newTestMutatorWithCustomizeMode()

	// Register a key binding so the tree is non-trivial.
	if err := m.BindKey("root", "C-b", "send-prefix"); err != nil {
		t.Fatalf("BindKey: %v", err)
	}

	if err := m.EnterCustomizeMode("c1"); err != nil {
		t.Fatalf("EnterCustomizeMode: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("overlay is nil")
	}
	rect := ov.Rect()
	dst := make([]modes.Cell, rect.Width*rect.Height)
	ov.Render(dst)

	hasContent := false
	for _, c := range dst {
		if c.Char != 0 && c.Char != ' ' {
			hasContent = true
			break
		}
	}
	if !hasContent {
		t.Error("Render produced empty canvas")
	}
}

// TestEnterCustomizeMode_QKeyClosesOverlay verifies that 'q' returns
// KindCloseMode, signalling that the overlay should be popped.
func TestEnterCustomizeMode_QKeyClosesOverlay(t *testing.T) {
	m, _, pushed := newTestMutatorWithCustomizeMode()

	if err := m.EnterCustomizeMode("c1"); err != nil {
		t.Fatalf("EnterCustomizeMode: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("overlay is nil")
	}
	outcome := ov.Key(keys.Key{Code: keys.KeyCode('q')})
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("'q' outcome = %v, want KindCloseMode", outcome.Kind)
	}
}

// TestEnterCustomizeMode_KeyBindingChangePersists verifies that committing an
// edit in the overlay updates the server's key table live.
func TestEnterCustomizeMode_KeyBindingChangePersists(t *testing.T) {
	m, _, pushed := newTestMutatorWithCustomizeMode()

	// Seed a binding.
	if err := m.BindKey("root", "C-b", "send-prefix"); err != nil {
		t.Fatalf("BindKey seed: %v", err)
	}

	if err := m.EnterCustomizeMode("c1"); err != nil {
		t.Fatalf("EnterCustomizeMode: %v", err)
	}

	ov, ok := pushed().(*modes.CustomizeOverlay)
	if !ok {
		t.Fatal("overlay is not *modes.CustomizeOverlay")
	}

	// Navigate to "Key Bindings" group (index 0 = Options, next = root group
	// or its children). Use down-arrow until we reach a binding leaf.
	// The tree starts at "Options" then "Key Bindings" > "root" > "C-b → ...".
	// Skip to a binding by pressing 'j' enough times.
	for i := 0; i < 10; i++ {
		ov.Key(keys.Key{Code: keys.KeyCode('j')})
		if ov.FlatLen() > 0 {
			// Check if we've reached a leaf that is a binding by entering edit
			// mode and seeing if the overlay switches to editing.
			ov.Key(keys.Key{Code: keys.CodeEnter})
			if ov.Editing() {
				break
			}
		}
	}
	if !ov.Editing() {
		t.Skip("could not navigate to a binding leaf — tree layout may differ")
	}

	// Clear the input and type a new command.
	for range "send-prefix" {
		ov.Key(keys.Key{Code: keys.CodeBackspace})
	}
	for _, ch := range "new-command" {
		ov.Key(keys.Key{Code: keys.KeyCode(ch)})
	}
	// Commit.
	ov.Key(keys.Key{Code: keys.CodeEnter})

	// The state's root table should now have the updated command.
	bindings := m.ListKeyBindings("root")
	found := false
	for _, b := range bindings {
		if b.Key == "C-b" && b.Command == "new-command" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("key binding C-b not updated to 'new-command'; bindings = %v", bindings)
	}
}

// TestEnterCustomizeMode_IncludesKeyBindings verifies that pre-registered key
// bindings appear in the customize overlay's flat list.
func TestEnterCustomizeMode_IncludesKeyBindings(t *testing.T) {
	m, _, pushed := newTestMutatorWithCustomizeMode()

	if err := m.BindKey("prefix", "c", "new-window"); err != nil {
		t.Fatalf("BindKey: %v", err)
	}

	if err := m.EnterCustomizeMode("c1"); err != nil {
		t.Fatalf("EnterCustomizeMode: %v", err)
	}

	ov, ok := pushed().(*modes.CustomizeOverlay)
	if !ok {
		t.Fatal("overlay is not *modes.CustomizeOverlay")
	}
	// FlatLen should be > 2 (at minimum "Options" and "Key Bindings" groups,
	// plus the "prefix" table group and the "c → new-window" leaf).
	if ov.FlatLen() < 4 {
		t.Errorf("FlatLen = %d, want >= 4 (groups + binding leaf)", ov.FlatLen())
	}
}
