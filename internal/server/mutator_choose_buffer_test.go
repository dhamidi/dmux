package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/command"
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	"github.com/dhamidi/dmux/internal/session"
)

// newTestMutatorWithChooseBuffer creates a serverMutator wired for
// choose-buffer tests. It builds a state with one session, one window, and one
// pane (backed by a recordingPane), adds two paste buffers, attaches client
// "c1", and returns tracking closures for overlay push events.
func newTestMutatorWithChooseBuffer() (
	m *serverMutator,
	rp *recordingPane,
	pushed func() modes.ClientOverlay,
) {
	state := session.NewServer()

	rp = &recordingPane{id: session.PaneID(1)}
	sess := session.NewSession(session.SessionID("s1"), "session1", nil)
	state.AddSession(sess)
	win := session.NewWindow(session.WindowID("w1"), "window1", nil)
	win.AddPane(session.PaneID(1), rp)
	win.Active = session.PaneID(1)
	wl := &session.Winlink{Index: 1, Window: win, Session: sess}
	sess.Windows = append(sess.Windows, wl)
	sess.Current = wl

	// Seed two paste buffers.
	state.Buffers.Set("buf0", "first buffer content")
	state.Buffers.Set("buf1", "second buffer content")

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

	return m, rp, func() modes.ClientOverlay { return lastOverlay }
}

// buildChooseBufferItems converts the server's buffer stack into the
// []command.ChooserItem slice that the builtin choose-buffer command would
// produce.
func buildChooseBufferItems(m *serverMutator) []command.ChooserItem {
	bufs := m.state.Buffers.List()
	items := make([]command.ChooserItem, len(bufs))
	for i, b := range bufs {
		items[i] = command.ChooserItem{
			Display: b.Name,
			Value:   b.Name,
		}
	}
	return items
}

// TestEnterChooseBuffer_AttachesOverlay verifies that EnterChooseBuffer pushes
// a ClientOverlay onto the target client.
func TestEnterChooseBuffer_AttachesOverlay(t *testing.T) {
	m, _, pushed := newTestMutatorWithChooseBuffer()
	items := buildChooseBufferItems(m)

	if err := m.EnterChooseBuffer("c1", "w1", items, "paste-buffer -b '%%'"); err != nil {
		t.Fatalf("EnterChooseBuffer: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("pushOverlayFn not called or overlay is nil")
	}
	if _, ok := ov.(*chooseBufferClientOverlay); !ok {
		t.Errorf("pushed overlay type = %T, want *chooseBufferClientOverlay", ov)
	}
}

// TestEnterChooseBuffer_OverlayCoversFullScreen verifies that the overlay rect
// matches the client's terminal size.
func TestEnterChooseBuffer_OverlayCoversFullScreen(t *testing.T) {
	m, _, pushed := newTestMutatorWithChooseBuffer()
	items := buildChooseBufferItems(m)

	if err := m.EnterChooseBuffer("c1", "w1", items, "paste-buffer -b '%%'"); err != nil {
		t.Fatalf("EnterChooseBuffer: %v", err)
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

// TestEnterChooseBuffer_ClientNotFound verifies that an error is returned when
// the client does not exist.
func TestEnterChooseBuffer_ClientNotFound(t *testing.T) {
	m, _, _ := newTestMutatorWithChooseBuffer()
	items := buildChooseBufferItems(m)

	if err := m.EnterChooseBuffer("no-such-client", "w1", items, "paste-buffer -b '%%'"); err == nil {
		t.Fatal("expected error for unknown client, got nil")
	}
}

// TestEnterChooseBuffer_BufferListPopulates verifies that the overlay's item
// list reflects the buffers passed by the caller.
func TestEnterChooseBuffer_BufferListPopulates(t *testing.T) {
	m, _, pushed := newTestMutatorWithChooseBuffer()
	items := buildChooseBufferItems(m)

	if err := m.EnterChooseBuffer("c1", "w1", items, "paste-buffer -b '%%'"); err != nil {
		t.Fatalf("EnterChooseBuffer: %v", err)
	}

	ov, ok := pushed().(*chooseBufferClientOverlay)
	if !ok {
		t.Fatal("overlay is not *chooseBufferClientOverlay")
	}
	if len(ov.items) != len(items) {
		t.Errorf("overlay item count = %d, want %d", len(ov.items), len(items))
	}
	for i, want := range items {
		if ov.items[i].Value != want.Value {
			t.Errorf("item[%d].Value = %q, want %q", i, ov.items[i].Value, want.Value)
		}
	}
}

// TestEnterChooseBuffer_SelectionPastesContent verifies that pressing Enter on
// a buffer item writes the buffer's data to the active pane.
func TestEnterChooseBuffer_SelectionPastesContent(t *testing.T) {
	m, rp, pushed := newTestMutatorWithChooseBuffer()
	items := buildChooseBufferItems(m)

	if err := m.EnterChooseBuffer("c1", "w1", items, "paste-buffer -b '%%'"); err != nil {
		t.Fatalf("EnterChooseBuffer: %v", err)
	}

	ov, ok := pushed().(*chooseBufferClientOverlay)
	if !ok {
		t.Fatal("overlay is not *chooseBufferClientOverlay")
	}

	// The list is ordered newest-first (buf1 was set last). Search for "buf0"
	// and press Enter to select it.
	ov.mode.SetSearch("buf0")
	outcome := ov.Key(keys.Key{Code: keys.CodeEnter})
	if outcome.Kind != modes.KindCloseMode {
		t.Fatalf("Enter outcome = %v, want KindCloseMode", outcome.Kind)
	}

	// The recording pane should have received the buffer data.
	got := string(rp.written)
	want := "first buffer content"
	if got != want {
		t.Errorf("pane written = %q, want %q", got, want)
	}
}

// TestEnterChooseBuffer_DKeyDeletesBuffer verifies that pressing 'd' removes
// the currently selected buffer from the server's buffer stack and from the
// overlay's item list.
func TestEnterChooseBuffer_DKeyDeletesBuffer(t *testing.T) {
	m, _, pushed := newTestMutatorWithChooseBuffer()
	items := buildChooseBufferItems(m)
	initialLen := len(items)

	if err := m.EnterChooseBuffer("c1", "w1", items, "paste-buffer -b '%%'"); err != nil {
		t.Fatalf("EnterChooseBuffer: %v", err)
	}

	ov, ok := pushed().(*chooseBufferClientOverlay)
	if !ok {
		t.Fatal("overlay is not *chooseBufferClientOverlay")
	}

	// Select "buf0" and delete it.
	ov.mode.SetSearch("buf0")
	outcome := ov.Key(keys.Key{Code: keys.KeyCode('d')})
	if outcome.Kind == modes.KindCloseMode {
		t.Fatal("'d' should not close the overlay")
	}

	// The overlay item list should be shorter.
	if len(ov.items) != initialLen-1 {
		t.Errorf("overlay items after delete = %d, want %d", len(ov.items), initialLen-1)
	}

	// The buffer should be gone from the server state.
	_, found := m.state.Buffers.GetNamed("buf0")
	if found {
		t.Error("buf0 still present in buffer stack after 'd' key")
	}
}

// TestEnterChooseBuffer_QKeyClosesOverlay verifies that 'q' signals KindCloseMode.
func TestEnterChooseBuffer_QKeyClosesOverlay(t *testing.T) {
	m, _, pushed := newTestMutatorWithChooseBuffer()
	items := buildChooseBufferItems(m)

	if err := m.EnterChooseBuffer("c1", "w1", items, "paste-buffer -b '%%'"); err != nil {
		t.Fatalf("EnterChooseBuffer: %v", err)
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
