package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/command"
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	"github.com/dhamidi/dmux/internal/session"
)

// newTestMutatorWithChooseClient creates a serverMutator wired for
// choose-client tests. It builds a state with two sessions, attaches two
// clients, and returns tracking closures for overlay push events.
func newTestMutatorWithChooseClient() (
	m *serverMutator,
	pushed func() modes.ClientOverlay,
) {
	state := session.NewServer()

	// Create two sessions.
	sess1 := session.NewSession(session.SessionID("s1"), "session1", nil)
	sess2 := session.NewSession(session.SessionID("s2"), "session2", nil)
	state.AddSession(sess1)
	state.AddSession(sess2)

	// Create the calling client (c1) attached to sess1.
	c1 := session.NewClient(session.ClientID("c1"))
	c1.Session = sess1
	c1.Size = session.Size{Rows: 24, Cols: 80}
	c1.TTY = "/dev/pts/0"
	state.Clients[c1.ID] = c1

	// Create a second client (c2) attached to sess2.
	c2 := session.NewClient(session.ClientID("c2"))
	c2.Session = sess2
	c2.Size = session.Size{Rows: 24, Cols: 80}
	c2.TTY = "/dev/pts/1"
	state.Clients[c2.ID] = c2

	var lastOverlay modes.ClientOverlay
	m = &serverMutator{
		state:    state,
		shutdown: func() {},
		pushOverlayFn: func(_ session.ClientID, ov modes.ClientOverlay) {
			lastOverlay = ov
		},
		popOverlayFn: func(_ session.ClientID) {},
	}

	return m, func() modes.ClientOverlay { return lastOverlay }
}

// buildChooseClientItems mirrors what the builtin choose-client command
// produces from the server's client list.
func buildChooseClientItems(m *serverMutator) []command.ChooserItem {
	var items []command.ChooserItem
	for _, c := range m.state.Clients {
		sessInfo := ""
		if c.Session != nil {
			sessInfo = string(c.Session.ID)
		}
		if sessInfo == "" {
			sessInfo = "(detached)"
		}
		items = append(items, command.ChooserItem{
			Display: string(c.ID) + " " + c.TTY + " " + sessInfo,
			Value:   string(c.ID),
		})
	}
	return items
}

// TestEnterChooseClient_AttachesOverlay verifies that EnterChooseClient pushes
// a ClientOverlay onto the target client.
func TestEnterChooseClient_AttachesOverlay(t *testing.T) {
	m, pushed := newTestMutatorWithChooseClient()
	items := buildChooseClientItems(m)

	if err := m.EnterChooseClient("c1", "w1", items, "switch-client -t '%%'"); err != nil {
		t.Fatalf("EnterChooseClient: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("pushOverlayFn not called or overlay is nil")
	}
	if _, ok := ov.(*chooseClientClientOverlay); !ok {
		t.Errorf("pushed overlay type = %T, want *chooseClientClientOverlay", ov)
	}
}

// TestEnterChooseClient_OverlayCoversFullScreen verifies that the overlay rect
// matches the client's terminal size.
func TestEnterChooseClient_OverlayCoversFullScreen(t *testing.T) {
	m, pushed := newTestMutatorWithChooseClient()
	items := buildChooseClientItems(m)

	if err := m.EnterChooseClient("c1", "w1", items, "switch-client -t '%%'"); err != nil {
		t.Fatalf("EnterChooseClient: %v", err)
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

// TestEnterChooseClient_ClientNotFound verifies that an error is returned when
// the client does not exist.
func TestEnterChooseClient_ClientNotFound(t *testing.T) {
	m, _ := newTestMutatorWithChooseClient()
	items := buildChooseClientItems(m)

	if err := m.EnterChooseClient("no-such-client", "w1", items, "switch-client -t '%%'"); err == nil {
		t.Fatal("expected error for unknown client, got nil")
	}
}

// TestEnterChooseClient_ClientListPopulates verifies that the overlay's item
// list reflects the items passed by the caller.
func TestEnterChooseClient_ClientListPopulates(t *testing.T) {
	m, pushed := newTestMutatorWithChooseClient()
	items := buildChooseClientItems(m)

	if err := m.EnterChooseClient("c1", "w1", items, "switch-client -t '%%'"); err != nil {
		t.Fatalf("EnterChooseClient: %v", err)
	}

	ov, ok := pushed().(*chooseClientClientOverlay)
	if !ok {
		t.Fatal("overlay is not *chooseClientClientOverlay")
	}
	if len(ov.items) != len(items) {
		t.Errorf("overlay item count = %d, want %d", len(ov.items), len(items))
	}
	// Verify all item values are present.
	seen := make(map[string]bool)
	for _, item := range ov.items {
		seen[item.Value] = true
	}
	for _, want := range items {
		if !seen[want.Value] {
			t.Errorf("item with value %q not found in overlay items", want.Value)
		}
	}
}

// TestEnterChooseClient_SelectionSwitchesSession verifies that selecting a
// client switches the calling client to that client's session.
func TestEnterChooseClient_SelectionSwitchesSession(t *testing.T) {
	m, pushed := newTestMutatorWithChooseClient()
	items := buildChooseClientItems(m)

	if err := m.EnterChooseClient("c1", "w1", items, "switch-client -t '%%'"); err != nil {
		t.Fatalf("EnterChooseClient: %v", err)
	}

	ov, ok := pushed().(*chooseClientClientOverlay)
	if !ok {
		t.Fatal("overlay is not *chooseClientClientOverlay")
	}

	// Select c2; c1 should be switched to s2.
	ov.mode.SetSearch("c2")
	outcome := ov.Key(keys.Key{Code: keys.CodeEnter})
	if outcome.Kind != modes.KindCloseMode {
		t.Fatalf("Enter outcome = %v, want KindCloseMode", outcome.Kind)
	}

	c1 := m.state.Clients[session.ClientID("c1")]
	if c1.Session == nil {
		t.Fatal("c1 session is nil after selection")
	}
	if c1.Session.ID != session.SessionID("s2") {
		t.Errorf("c1 session after selection = %q, want %q", c1.Session.ID, "s2")
	}
}

// TestEnterChooseClient_DKeyDetachesClient verifies that pressing 'd' removes
// the selected client from the server's clients map and from the overlay's
// item list.
func TestEnterChooseClient_DKeyDetachesClient(t *testing.T) {
	m, pushed := newTestMutatorWithChooseClient()
	items := buildChooseClientItems(m)
	initialLen := len(items)

	if err := m.EnterChooseClient("c1", "w1", items, "switch-client -t '%%'"); err != nil {
		t.Fatalf("EnterChooseClient: %v", err)
	}

	ov, ok := pushed().(*chooseClientClientOverlay)
	if !ok {
		t.Fatal("overlay is not *chooseClientClientOverlay")
	}

	// Select c2 and detach it.
	ov.mode.SetSearch("c2")
	outcome := ov.Key(keys.Key{Code: keys.KeyCode('d')})
	if outcome.Kind == modes.KindCloseMode {
		t.Fatal("'d' should not close the overlay")
	}

	// The overlay item list should be shorter.
	if len(ov.items) != initialLen-1 {
		t.Errorf("overlay items after detach = %d, want %d", len(ov.items), initialLen-1)
	}

	// c2 should be gone from the server state.
	if _, found := m.state.Clients[session.ClientID("c2")]; found {
		t.Error("c2 still present in Clients map after 'd' key")
	}
}

// TestEnterChooseClient_QKeyClosesOverlay verifies that 'q' signals KindCloseMode.
func TestEnterChooseClient_QKeyClosesOverlay(t *testing.T) {
	m, pushed := newTestMutatorWithChooseClient()
	items := buildChooseClientItems(m)

	if err := m.EnterChooseClient("c1", "w1", items, "switch-client -t '%%'"); err != nil {
		t.Fatalf("EnterChooseClient: %v", err)
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
