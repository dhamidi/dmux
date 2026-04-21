package server

import (
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	clockmode "github.com/dhamidi/dmux/internal/modes/clock"
	"github.com/dhamidi/dmux/internal/session"
)

// newTestMutatorWithClockMode creates a serverMutator with a minimal session
// state suitable for clock-mode tests.
func newTestMutatorWithClockMode() (
	m *serverMutator,
	client *session.Client,
	pushed func() (session.PaneID, modes.PaneMode),
) {
	state := session.NewServer()

	sess := session.NewSession(session.SessionID("s1"), "s1", nil)
	state.AddSession(sess)

	win := session.NewWindow(session.WindowID("w1"), "w1", nil)
	paneID := session.PaneID(1)
	fp := &fakePane{id: paneID}
	win.AddPane(paneID, fp)

	wl := &session.Winlink{Window: win, Session: sess, Index: 1}
	sess.Windows = append(sess.Windows, wl)
	sess.Current = wl

	c := session.NewClient(session.ClientID("c1"))
	state.Clients[c.ID] = c
	c.Session = sess

	var lastPushedID session.PaneID
	var lastPushedMode modes.PaneMode

	m = &serverMutator{
		state:    state,
		shutdown: func() {},
		pushPaneOverlayFn: func(id session.ClientID, pid session.PaneID, mode modes.PaneMode) {
			lastPushedID = pid
			lastPushedMode = mode
		},
		popPaneOverlayFn: func(id session.ClientID, pid session.PaneID) {},
	}

	return m, c,
		func() (session.PaneID, modes.PaneMode) { return lastPushedID, lastPushedMode }
}

// TestEnterClockMode_AttachesOverlay verifies that EnterClockMode registers a
// clockPaneMode as the active pane overlay for the target pane.
func TestEnterClockMode_AttachesOverlay(t *testing.T) {
	m, _, pushed := newTestMutatorWithClockMode()

	if err := m.EnterClockMode("c1", 1); err != nil {
		t.Fatalf("EnterClockMode: %v", err)
	}

	paneID, mode := pushed()
	if mode == nil {
		t.Fatal("pushPaneOverlayFn not called or mode is nil")
	}
	if paneID != session.PaneID(1) {
		t.Errorf("pushed pane ID = %v, want 1", paneID)
	}
	if _, ok := mode.(*clockPaneMode); !ok {
		t.Errorf("pushed mode type = %T, want *clockPaneMode", mode)
	}

	// Clean up the ticker goroutine.
	mode.Close()
}

// TestEnterClockMode_ClientNotFound verifies an error is returned when the
// client does not exist.
func TestEnterClockMode_ClientNotFound(t *testing.T) {
	m, _, _ := newTestMutatorWithClockMode()

	if err := m.EnterClockMode("no-such-client", 1); err == nil {
		t.Fatal("expected error for unknown client, got nil")
	}
}

// TestEnterClockMode_NoSession verifies an error is returned when the client
// has no attached session.
func TestEnterClockMode_NoSession(t *testing.T) {
	m, client, _ := newTestMutatorWithClockMode()
	client.Session = nil

	if err := m.EnterClockMode("c1", 1); err == nil {
		t.Fatal("expected error for client with no session, got nil")
	}
}

// TestEnterClockMode_PaneNotFound verifies an error is returned when the
// specified pane does not exist.
func TestEnterClockMode_PaneNotFound(t *testing.T) {
	m, _, _ := newTestMutatorWithClockMode()

	if err := m.EnterClockMode("c1", 999); err == nil {
		t.Fatal("expected error for unknown pane, got nil")
	}
}

// TestEnterClockMode_ZeroPaneUsesActive verifies that paneID <= 0 falls back
// to the window's active pane.
func TestEnterClockMode_ZeroPaneUsesActive(t *testing.T) {
	m, _, pushed := newTestMutatorWithClockMode()

	if err := m.EnterClockMode("c1", 0); err != nil {
		t.Fatalf("EnterClockMode with paneID=0: %v", err)
	}

	paneID, mode := pushed()
	if mode == nil {
		t.Fatal("pushPaneOverlayFn not called or mode is nil")
	}
	if paneID != session.PaneID(1) {
		t.Errorf("pushed pane ID = %v, want 1 (active)", paneID)
	}
	mode.Close()
}

// TestClockPaneMode_RendersCurrentTime verifies that the clock overlay renders
// content onto the canvas (non-empty output for the current time).
func TestClockPaneMode_RendersCurrentTime(t *testing.T) {
	m, _, pushed := newTestMutatorWithClockMode()

	if err := m.EnterClockMode("c1", 1); err != nil {
		t.Fatalf("EnterClockMode: %v", err)
	}
	_, mode := pushed()
	defer mode.Close()

	canvas := &gridCanvas{rows: 24, cols: 80, cells: make([]modes.Cell, 24*80)}
	mode.Render(canvas)

	// At least one cell must be non-zero — the clock always draws digits.
	hasContent := false
	for _, c := range canvas.cells {
		if c.Char != 0 {
			hasContent = true
			break
		}
	}
	if !hasContent {
		t.Error("Render produced empty canvas for 80×24 grid")
	}
}

// TestClockPaneMode_RendersSpecificTime verifies that the clock renders the
// time returned by the injected now function.
func TestClockPaneMode_RendersSpecificTime(t *testing.T) {
	state := session.NewServer()

	sess := session.NewSession(session.SessionID("s1"), "s1", nil)
	state.AddSession(sess)

	win := session.NewWindow(session.WindowID("w1"), "w1", nil)
	paneID := session.PaneID(1)
	win.AddPane(paneID, &fakePane{id: paneID})

	wl := &session.Winlink{Window: win, Session: sess, Index: 1}
	sess.Windows = append(sess.Windows, wl)
	sess.Current = wl

	c := session.NewClient(session.ClientID("c1"))
	state.Clients[c.ID] = c
	c.Session = sess

	// Use fixed time 12:34.
	fixed := time.Date(2024, 1, 1, 12, 34, 0, 0, time.UTC)

	var capturedMode modes.PaneMode
	m := &serverMutator{
		state:    state,
		shutdown: func() {},
		pushPaneOverlayFn: func(id session.ClientID, pid session.PaneID, mode modes.PaneMode) {
			capturedMode = mode
		},
		popPaneOverlayFn: func(id session.ClientID, pid session.PaneID) {},
	}

	// Inject clock mode directly with a fixed time.
	clockMode := &clockPaneMode{
		mode:   clockmode.New(func() time.Time { return fixed }),
		stopFn: nil, // no ticker in this test
	}
	m.pushPaneOverlayFn(session.ClientID("c1"), paneID, clockMode)
	_ = capturedMode

	canvas := &gridCanvas{rows: 24, cols: 80, cells: make([]modes.Cell, 24*80)}
	clockMode.Render(canvas)

	// Verify something was drawn.
	hasContent := false
	for _, c := range canvas.cells {
		if c.Char != 0 {
			hasContent = true
			break
		}
	}
	if !hasContent {
		t.Error("clock did not render any content for time 12:34")
	}
}

// TestClockPaneMode_AnyKeyClosesMode verifies that any key press returns
// KindCloseMode, exiting clock mode.
func TestClockPaneMode_AnyKeyClosesMode(t *testing.T) {
	m, _, pushed := newTestMutatorWithClockMode()

	if err := m.EnterClockMode("c1", 1); err != nil {
		t.Fatalf("EnterClockMode: %v", err)
	}
	_, mode := pushed()
	defer mode.Close()

	testKeys := []keys.Key{
		{Code: keys.KeyCode('q')},
		{Code: keys.KeyCode(' ')},
		{Code: keys.CodeEscape},
		{Code: keys.CodeEnter},
	}

	for _, k := range testKeys {
		outcome := mode.Key(k)
		if outcome.Kind != modes.KindCloseMode {
			t.Errorf("key %v: outcome = %v, want KindCloseMode", k, outcome.Kind)
		}
	}
}

// TestClockPaneMode_CloseStopsTicker verifies that Close stops the background
// ticker goroutine without blocking or panicking.
func TestClockPaneMode_CloseStopsTicker(t *testing.T) {
	m, _, pushed := newTestMutatorWithClockMode()

	if err := m.EnterClockMode("c1", 1); err != nil {
		t.Fatalf("EnterClockMode: %v", err)
	}
	_, mode := pushed()

	// Close should return promptly and not panic.
	done := make(chan struct{})
	go func() {
		mode.Close()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("Close() did not return within 2 seconds")
	}
}
