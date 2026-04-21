package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/command"
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	promptmode "github.com/dhamidi/dmux/internal/modes/prompt"
	"github.com/dhamidi/dmux/internal/session"
)

// newTestMutatorForConfirmBefore sets up a serverMutator suitable for
// confirm-before tests. It creates one session/window/pane, attaches client
// "c1", wires overlay tracking, and returns helpers to inspect push/pop calls.
func newTestMutatorForConfirmBefore() (
	m *serverMutator,
	client *session.Client,
	pushed func() modes.ClientOverlay,
	popped func() bool,
) {
	state := session.NewServer()

	sess := session.NewSession(session.SessionID("s1"), "session1", nil)
	state.AddSession(sess)
	win := session.NewWindow(session.WindowID("w1"), "window1", nil)
	win.AddPane(session.PaneID(1), &fakePane{id: session.PaneID(1)})
	wl := &session.Winlink{Index: 1, Window: win, Session: sess}
	sess.Windows = append(sess.Windows, wl)
	sess.Current = wl

	c := session.NewClient(session.ClientID("c1"))
	c.Session = sess
	c.Size = session.Size{Rows: 24, Cols: 80}
	state.Clients[c.ID] = c

	var lastOverlay modes.ClientOverlay
	var popCalled bool

	m = &serverMutator{
		state:    state,
		queue:    command.NewQueue(),
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

// TestConfirmBefore_AttachesOverlay verifies that ConfirmBefore pushes a
// ConfirmMode overlay onto the target client.
func TestConfirmBefore_AttachesOverlay(t *testing.T) {
	m, _, pushed, _ := newTestMutatorForConfirmBefore()

	if err := m.ConfirmBefore("c1", "Kill server?", "kill-server"); err != nil {
		t.Fatalf("ConfirmBefore: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("pushOverlayFn not called or overlay is nil")
	}
	if _, ok := ov.(*promptmode.ConfirmMode); !ok {
		t.Errorf("pushed overlay type = %T, want *promptmode.ConfirmMode", ov)
	}
}

// TestConfirmBefore_ClientNotFound verifies an error is returned when the
// client does not exist.
func TestConfirmBefore_ClientNotFound(t *testing.T) {
	m, _, _, _ := newTestMutatorForConfirmBefore()

	if err := m.ConfirmBefore("no-such-client", "Delete?", "kill-server"); err == nil {
		t.Fatal("expected error for unknown client, got nil")
	}
}

// TestConfirmBefore_YEnqueuesCommand verifies that pressing y with a valid
// command enqueues that command in the queue.
func TestConfirmBefore_YEnqueuesCommand(t *testing.T) {
	m, _, pushed, _ := newTestMutatorForConfirmBefore()

	if err := m.ConfirmBefore("c1", "Kill server?", "kill-server"); err != nil {
		t.Fatalf("ConfirmBefore: %v", err)
	}

	ov, ok := pushed().(*promptmode.ConfirmMode)
	if !ok {
		t.Fatal("overlay is not *promptmode.ConfirmMode")
	}

	queueBefore := m.queue.Len()

	outcome := ov.Key(keys.Key{Code: keys.KeyCode('y')})
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("y outcome = %v, want KindCloseMode", outcome.Kind)
	}

	if got := m.queue.Len(); got <= queueBefore {
		t.Errorf("queue length after y = %d, want > %d", got, queueBefore)
	}
}

// TestConfirmBefore_NDiscardsWithoutEnqueuing verifies that pressing n closes
// the overlay without enqueuing any command.
func TestConfirmBefore_NDiscardsWithoutEnqueuing(t *testing.T) {
	m, _, pushed, _ := newTestMutatorForConfirmBefore()

	if err := m.ConfirmBefore("c1", "Kill server?", "kill-server"); err != nil {
		t.Fatalf("ConfirmBefore: %v", err)
	}

	ov, ok := pushed().(*promptmode.ConfirmMode)
	if !ok {
		t.Fatal("overlay is not *promptmode.ConfirmMode")
	}

	queueBefore := m.queue.Len()

	outcome := ov.Key(keys.Key{Code: keys.KeyCode('n')})
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("n outcome = %v, want KindCloseMode", outcome.Kind)
	}

	if got := m.queue.Len(); got != queueBefore {
		t.Errorf("queue length after n = %d, want %d (no new items)", got, queueBefore)
	}
}

// TestConfirmBefore_EscapeDiscardsWithoutEnqueuing verifies that pressing
// Escape closes the overlay without enqueuing any command.
func TestConfirmBefore_EscapeDiscardsWithoutEnqueuing(t *testing.T) {
	m, _, pushed, _ := newTestMutatorForConfirmBefore()

	if err := m.ConfirmBefore("c1", "Kill server?", "kill-server"); err != nil {
		t.Fatalf("ConfirmBefore: %v", err)
	}

	ov, ok := pushed().(*promptmode.ConfirmMode)
	if !ok {
		t.Fatal("overlay is not *promptmode.ConfirmMode")
	}

	queueBefore := m.queue.Len()

	outcome := ov.Key(keys.Key{Code: keys.CodeEscape})
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("Escape outcome = %v, want KindCloseMode", outcome.Kind)
	}

	if got := m.queue.Len(); got != queueBefore {
		t.Errorf("queue length after Escape = %d, want %d (no new items)", got, queueBefore)
	}
}

// TestConfirmBefore_OverlayAtBottomRow verifies that the overlay rect is
// positioned at the last row of the client's terminal.
func TestConfirmBefore_OverlayAtBottomRow(t *testing.T) {
	m, _, pushed, _ := newTestMutatorForConfirmBefore()

	if err := m.ConfirmBefore("c1", "Kill server?", "kill-server"); err != nil {
		t.Fatalf("ConfirmBefore: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("overlay is nil")
	}
	rect := ov.Rect()
	if rect.Width != 80 || rect.Height != 1 {
		t.Errorf("overlay rect size = {%d×%d}, want {80×1}", rect.Width, rect.Height)
	}
	if rect.Y != 23 {
		t.Errorf("overlay rect Y = %d, want 23 (last row)", rect.Y)
	}
}
