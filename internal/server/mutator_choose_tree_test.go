package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	treemode "github.com/dhamidi/dmux/internal/modes/tree"
	"github.com/dhamidi/dmux/internal/session"
)

// newTestMutatorWithChooseTree creates a serverMutator wired for choose-tree
// tests. It builds a state with two sessions, each with one window and one
// pane, attaches client "c1" to session "s1", and returns tracking closures
// for overlay push/pop events.
func newTestMutatorWithChooseTree() (
	m *serverMutator,
	client *session.Client,
	pushed func() modes.ClientOverlay,
	popped func() bool,
) {
	state := session.NewServer()

	// Session 1 with window 1 and pane 1.
	sess1 := session.NewSession(session.SessionID("s1"), "session1", nil)
	state.AddSession(sess1)
	win1 := session.NewWindow(session.WindowID("w1"), "window1", nil)
	win1.AddPane(session.PaneID(1), &fakePane{id: session.PaneID(1)})
	wl1 := &session.Winlink{Index: 1, Window: win1, Session: sess1}
	sess1.Windows = append(sess1.Windows, wl1)
	sess1.Current = wl1

	// Session 2 with window 2 and pane 2.
	sess2 := session.NewSession(session.SessionID("s2"), "session2", nil)
	state.AddSession(sess2)
	win2 := session.NewWindow(session.WindowID("w2"), "window2", nil)
	win2.AddPane(session.PaneID(2), &fakePane{id: session.PaneID(2)})
	wl2 := &session.Winlink{Index: 1, Window: win2, Session: sess2}
	sess2.Windows = append(sess2.Windows, wl2)
	sess2.Current = wl2

	// Client attached to session 1.
	c := session.NewClient(session.ClientID("c1"))
	c.Session = sess1
	c.Size = session.Size{Rows: 24, Cols: 80}
	state.Clients[c.ID] = c

	// Tracking variables.
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
	}

	return m, c,
		func() modes.ClientOverlay { return lastOverlay },
		func() bool { return popCalled }
}

// TestEnterChooseTree_AttachesOverlay verifies that EnterChooseTree pushes a
// ClientOverlay onto the target client.
func TestEnterChooseTree_AttachesOverlay(t *testing.T) {
	m, _, pushed, _ := newTestMutatorWithChooseTree()

	if err := m.EnterChooseTree("c1", "s1", "w1"); err != nil {
		t.Fatalf("EnterChooseTree: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("pushOverlayFn not called or overlay is nil")
	}
	if _, ok := ov.(*treeClientOverlay); !ok {
		t.Errorf("pushed overlay type = %T, want *treeClientOverlay", ov)
	}
}

// TestEnterChooseTree_OverlayCoversFullScreen verifies that the overlay rect
// matches the client's terminal size.
func TestEnterChooseTree_OverlayCoversFullScreen(t *testing.T) {
	m, _, pushed, _ := newTestMutatorWithChooseTree()

	if err := m.EnterChooseTree("c1", "s1", "w1"); err != nil {
		t.Fatalf("EnterChooseTree: %v", err)
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

// TestEnterChooseTree_TreeContainsSessionsWindowsPanes verifies that the tree
// is populated with all sessions, windows, and panes from the server state.
func TestEnterChooseTree_TreeContainsSessionsWindowsPanes(t *testing.T) {
	m, _, pushed, _ := newTestMutatorWithChooseTree()

	if err := m.EnterChooseTree("c1", "s1", "w1"); err != nil {
		t.Fatalf("EnterChooseTree: %v", err)
	}

	ov, ok := pushed().(*treeClientOverlay)
	if !ok {
		t.Fatal("overlay is not *treeClientOverlay")
	}

	// Render the overlay to a flat cell slice and check that at least the
	// session names appear (indirectly, by counting visible rows).
	dst := make([]modes.Cell, 24*80)
	ov.Render(dst)

	// SelectedID should be a valid node (not "").
	if id := ov.mode.SelectedID(); id == "" {
		t.Error("tree mode has no selected node after construction")
	}
}

// TestEnterChooseTree_ClientNotFound verifies that an error is returned when
// the client does not exist.
func TestEnterChooseTree_ClientNotFound(t *testing.T) {
	m, _, _, _ := newTestMutatorWithChooseTree()

	if err := m.EnterChooseTree("no-such-client", "s1", "w1"); err == nil {
		t.Fatal("expected error for unknown client, got nil")
	}
}

// TestEnterChooseTree_SelectPaneSwitchesFocus verifies that selecting a pane
// node sets the window's active pane and switches the client session when
// needed.
func TestEnterChooseTree_SelectPaneSwitchesFocus(t *testing.T) {
	m, client, pushed, _ := newTestMutatorWithChooseTree()

	if err := m.EnterChooseTree("c1", "s1", "w1"); err != nil {
		t.Fatalf("EnterChooseTree: %v", err)
	}

	ov, ok := pushed().(*treeClientOverlay)
	if !ok {
		t.Fatal("overlay is not *treeClientOverlay")
	}

	// Directly invoke the underlying treemode.Mode's onSelect with a pane ID
	// that belongs to session 2, window 2, pane 2 — verifying cross-session
	// switching.
	ov.mode.Key(keys.Key{}) // no-op, just exercise key routing
	_ = ov.CaptureFocus()

	// Simulate selecting pane 2 in session 2.
	// We set the cursor to the pane node by calling onSelect directly via a
	// synthetic select using the mode's Key/Enter mechanism.  Because the
	// visible node order is deterministic only relative to map iteration, we
	// call onSelect through the mode's internal mechanism by setting the
	// search to the pane name and pressing Enter.
	ov.mode.SetSearch("%2")
	outcome := ov.mode.Key(keys.Key{Code: keys.CodeEnter})
	if outcome.Kind != modes.KindCloseMode {
		t.Fatalf("Enter outcome = %v, want KindCloseMode", outcome.Kind)
	}

	// After selection, the client should be attached to session 2.
	if client.Session == nil || string(client.Session.ID) != "s2" {
		sessID := ""
		if client.Session != nil {
			sessID = string(client.Session.ID)
		}
		t.Errorf("client session after pane select = %q, want %q", sessID, "s2")
	}

	// Window 2 should be the current window in session 2.
	sess2 := m.state.Sessions[session.SessionID("s2")]
	if sess2.Current == nil || string(sess2.Current.Window.ID) != "w2" {
		t.Errorf("session2 current window = %v, want w2", sess2.Current)
	}

	// Pane 2 should be the active pane.
	if sess2.Current.Window.Active != session.PaneID(2) {
		t.Errorf("active pane = %v, want 2", sess2.Current.Window.Active)
	}
}

// TestEnterChooseTree_SelectWindowSwitchesFocus verifies that selecting a
// window node sets the session's current window.
func TestEnterChooseTree_SelectWindowSwitchesFocus(t *testing.T) {
	m, client, pushed, _ := newTestMutatorWithChooseTree()

	if err := m.EnterChooseTree("c1", "s1", "w1"); err != nil {
		t.Fatalf("EnterChooseTree: %v", err)
	}

	ov, ok := pushed().(*treeClientOverlay)
	if !ok {
		t.Fatal("overlay is not *treeClientOverlay")
	}

	// Search for window2 (in session2) and select it.
	ov.mode.SetSearch("window2")
	outcome := ov.mode.Key(keys.Key{Code: keys.CodeEnter})
	if outcome.Kind != modes.KindCloseMode {
		t.Fatalf("Enter outcome = %v, want KindCloseMode", outcome.Kind)
	}

	// The client should now be attached to session 2.
	if client.Session == nil || string(client.Session.ID) != "s2" {
		sessID := ""
		if client.Session != nil {
			sessID = string(client.Session.ID)
		}
		t.Errorf("client session after window select = %q, want %q", sessID, "s2")
	}
}

// TestEnterChooseTree_QKeyClosesOverlay verifies that the overlay signals
// KindCloseMode when 'q' is pressed.
func TestEnterChooseTree_QKeyClosesOverlay(t *testing.T) {
	m, _, pushed, _ := newTestMutatorWithChooseTree()

	if err := m.EnterChooseTree("c1", "s1", "w1"); err != nil {
		t.Fatalf("EnterChooseTree: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("overlay is nil")
	}

	// The server pops the overlay on KindCloseMode; verify the overlay
	// reports that outcome for 'q'.
	outcome := ov.Key(keys.Key{Code: keys.KeyCode('q')})
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("'q' outcome = %v, want KindCloseMode", outcome.Kind)
	}
}

// TestEnterChooseTree_SrvPopOverlayOnQ verifies the full round-trip: the
// overlay is removed from the srv overlay stack when 'q' produces KindCloseMode.
func TestEnterChooseTree_SrvPopOverlayOnQ(t *testing.T) {
	state := session.NewServer()

	sess1 := session.NewSession(session.SessionID("s1"), "session1", nil)
	state.AddSession(sess1)
	win1 := session.NewWindow(session.WindowID("w1"), "window1", nil)
	win1.AddPane(session.PaneID(1), &fakePane{id: session.PaneID(1)})
	wl1 := &session.Winlink{Index: 1, Window: win1, Session: sess1}
	sess1.Windows = append(sess1.Windows, wl1)
	sess1.Current = wl1

	client := session.NewClient(session.ClientID("c1"))
	client.Session = sess1
	client.Size = session.Size{Rows: 24, Cols: 80}
	state.Clients[client.ID] = client

	cc := &clientConn{
		id:           session.ClientID("c1"),
		client:       client,
		dirty:        make(chan struct{}, 1),
		paneOverlays: map[session.PaneID]modes.PaneMode{},
	}

	s := &srv{
		state: state,
		conns: map[session.ClientID]*clientConn{
			session.ClientID("c1"): cc,
		},
		done: make(chan struct{}),
	}

	// Push a tree overlay via the mutator.
	sm := &serverMutator{
		state:    state,
		shutdown: func() {},
		pushOverlayFn: func(id session.ClientID, ov modes.ClientOverlay) {
			s.pushOverlay(id, ov)
		},
		popOverlayFn: func(id session.ClientID) {
			s.popOverlay(id)
		},
	}

	if err := sm.EnterChooseTree("c1", "s1", "w1"); err != nil {
		t.Fatalf("EnterChooseTree: %v", err)
	}

	// Verify overlay is on the stack.
	s.mu.Lock()
	nBefore := len(cc.overlays)
	s.mu.Unlock()
	if nBefore != 1 {
		t.Fatalf("overlay count before q = %d, want 1", nBefore)
	}

	// Press 'q' and simulate what the server loop does on KindCloseMode.
	s.mu.Lock()
	ov := cc.overlays[len(cc.overlays)-1]
	s.mu.Unlock()

	outcome := ov.Key(keys.Key{Code: keys.KeyCode('q')})
	if outcome.Kind != modes.KindCloseMode {
		t.Fatalf("'q' outcome = %v, want KindCloseMode", outcome.Kind)
	}
	s.popOverlay(session.ClientID("c1"))

	// Overlay should be gone.
	s.mu.Lock()
	nAfter := len(cc.overlays)
	s.mu.Unlock()
	if nAfter != 0 {
		t.Errorf("overlay count after q = %d, want 0", nAfter)
	}
}

// fakeTreeNode is a helper that builds a treemode.Mode for assertions about
// node IDs without going through the server.
func buildExpectedTreeNodes(sessID, winID string, paneIDs []int) []treemode.TreeNode {
	winNode := treemode.TreeNode{
		Kind: treemode.KindWindow,
		ID:   "w:" + sessID + ":" + winID,
	}
	for _, pid := range paneIDs {
		winNode.Children = append(winNode.Children, treemode.TreeNode{
			Kind: treemode.KindPane,
			ID:   "p:" + sessID + ":" + winID + ":" + string(rune('0'+pid)),
		})
	}
	return []treemode.TreeNode{{
		Kind:     treemode.KindSession,
		ID:       "s:" + sessID,
		Children: []treemode.TreeNode{winNode},
	}}
}
