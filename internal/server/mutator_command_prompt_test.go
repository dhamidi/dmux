package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/command"
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	promptmode "github.com/dhamidi/dmux/internal/modes/prompt"
	"github.com/dhamidi/dmux/internal/session"
)


// newTestMutatorWithCommandPrompt sets up a serverMutator suitable for
// command-prompt tests. It creates one session/window/pane, attaches client
// "c1", wires overlay tracking, and returns helpers to inspect push/pop calls.
func newTestMutatorWithCommandPrompt() (
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

// TestCommandPrompt_AttachesOverlay verifies that CommandPrompt pushes a
// ClientOverlay onto the target client.
func TestCommandPrompt_AttachesOverlay(t *testing.T) {
	m, _, pushed, _ := newTestMutatorWithCommandPrompt()

	if err := m.CommandPrompt("c1", ":", ""); err != nil {
		t.Fatalf("CommandPrompt: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("pushOverlayFn not called or overlay is nil")
	}
	if _, ok := ov.(*promptmode.CommandMode); !ok {
		t.Errorf("pushed overlay type = %T, want *promptmode.CommandMode", ov)
	}
}

// TestCommandPrompt_OverlayAtBottomRow verifies that the overlay rect is
// positioned at the last row of the client's terminal.
func TestCommandPrompt_OverlayAtBottomRow(t *testing.T) {
	m, _, pushed, _ := newTestMutatorWithCommandPrompt()

	if err := m.CommandPrompt("c1", ":", ""); err != nil {
		t.Fatalf("CommandPrompt: %v", err)
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

// TestCommandPrompt_DefaultPrompt verifies that an empty prompt string is
// replaced with the default ":".
func TestCommandPrompt_DefaultPrompt(t *testing.T) {
	m, _, pushed, _ := newTestMutatorWithCommandPrompt()

	if err := m.CommandPrompt("c1", "", ""); err != nil {
		t.Fatalf("CommandPrompt: %v", err)
	}

	ov, ok := pushed().(*promptmode.CommandMode)
	if !ok {
		t.Fatal("overlay is not *promptmode.CommandMode")
	}

	// Render a single-row prompt and verify ":" appears.
	dst := make([]modes.Cell, 80)
	ov.Render(dst)
	if dst[0].Char != ':' {
		t.Errorf("first cell = %q, want ':'", dst[0].Char)
	}
}

// TestCommandPrompt_InitialText verifies that the initial text is pre-populated
// in the input buffer.
func TestCommandPrompt_InitialText(t *testing.T) {
	m, _, pushed, _ := newTestMutatorWithCommandPrompt()

	if err := m.CommandPrompt("c1", ":", "hello"); err != nil {
		t.Fatalf("CommandPrompt: %v", err)
	}

	ov, ok := pushed().(*promptmode.CommandMode)
	if !ok {
		t.Fatal("overlay is not *promptmode.CommandMode")
	}

	if got := ov.Input(); got != "hello" {
		t.Errorf("initial input = %q, want %q", got, "hello")
	}
}

// TestCommandPrompt_ClientNotFound verifies an error is returned when the
// client does not exist.
func TestCommandPrompt_ClientNotFound(t *testing.T) {
	m, _, _, _ := newTestMutatorWithCommandPrompt()

	if err := m.CommandPrompt("no-such-client", ":", ""); err == nil {
		t.Fatal("expected error for unknown client, got nil")
	}
}

// TestCommandPrompt_SubmitEnqueuesCommand verifies that pressing Enter with
// a valid command enqueues that command in the queue.
func TestCommandPrompt_SubmitEnqueuesCommand(t *testing.T) {
	m, _, pushed, _ := newTestMutatorWithCommandPrompt()

	if err := m.CommandPrompt("c1", ":", "new-window"); err != nil {
		t.Fatalf("CommandPrompt: %v", err)
	}

	ov, ok := pushed().(*promptmode.CommandMode)
	if !ok {
		t.Fatal("overlay is not *promptmode.CommandMode")
	}

	queueBefore := m.queue.Len()

	// Press Enter to submit.
	outcome := ov.Key(keys.Key{Code: keys.CodeEnter})
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("Enter outcome = %v, want KindCloseMode", outcome.Kind)
	}

	if got := m.queue.Len(); got <= queueBefore {
		t.Errorf("queue length after Enter = %d, want > %d", got, queueBefore)
	}
}

// TestCommandPrompt_EscapeClosesWithoutAction verifies that pressing Escape
// closes the overlay without enqueuing any command.
func TestCommandPrompt_EscapeClosesWithoutAction(t *testing.T) {
	m, _, pushed, _ := newTestMutatorWithCommandPrompt()

	if err := m.CommandPrompt("c1", ":", "new-window"); err != nil {
		t.Fatalf("CommandPrompt: %v", err)
	}

	ov, ok := pushed().(*promptmode.CommandMode)
	if !ok {
		t.Fatal("overlay is not *promptmode.CommandMode")
	}

	queueBefore := m.queue.Len()

	// Press Escape to cancel.
	outcome := ov.Key(keys.Key{Code: keys.CodeEscape})
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("Escape outcome = %v, want KindCloseMode", outcome.Kind)
	}

	if got := m.queue.Len(); got != queueBefore {
		t.Errorf("queue length after Escape = %d, want %d (no new items)", got, queueBefore)
	}
}
