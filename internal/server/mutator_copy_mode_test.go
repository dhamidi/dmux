package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	copymode "github.com/dhamidi/dmux/internal/modes/copy"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/session"
)

// scrollablePane is a fakePane that returns preset lines from Snapshot
// so copy-mode has something to display.
type scrollablePane struct {
	fakePane
	grid pane.CellGrid
}

func newScrollablePane(id session.PaneID, rows, cols int, text string) *scrollablePane {
	cells := make([]pane.Cell, rows*cols)
	for i, ch := range text {
		if i < len(cells) {
			cells[i] = pane.Cell{Char: ch}
		}
	}
	return &scrollablePane{
		fakePane: fakePane{id: id},
		grid:     pane.CellGrid{Rows: rows, Cols: cols, Cells: cells},
	}
}

func (p *scrollablePane) Snapshot() pane.CellGrid { return p.grid }

// newTestMutatorWithCopyMode creates a serverMutator that tracks
// pushPaneOverlayFn / popPaneOverlayFn calls and the session state needed for
// copy-mode tests.
func newTestMutatorWithCopyMode() (
	m *serverMutator,
	client *session.Client,
	pushed func() (session.PaneID, modes.PaneMode),
	popped func() (session.PaneID, bool),
) {
	state := session.NewServer()

	// Create session, window, and pane.
	sess := session.NewSession(session.SessionID("s1"), "s1", nil)
	state.AddSession(sess)

	win := session.NewWindow(session.WindowID("w1"), "w1", nil)
	paneID := session.PaneID(1)
	sp := newScrollablePane(paneID, 10, 40, "hello world")
	win.AddPane(paneID, sp)

	wl := &session.Winlink{Window: win, Session: sess, Index: 1}
	sess.Windows = append(sess.Windows, wl)
	sess.Current = wl

	c := session.NewClient(session.ClientID("c1"))
	state.Clients[c.ID] = c
	c.Session = sess

	// Track overlay push/pop calls.
	var lastPushedID session.PaneID
	var lastPushedMode modes.PaneMode
	var poppedID session.PaneID
	var poppedCalled bool

	m = &serverMutator{
		state:    state,
		shutdown: func() {},
		pushPaneOverlayFn: func(id session.ClientID, pid session.PaneID, mode modes.PaneMode) {
			lastPushedID = pid
			lastPushedMode = mode
		},
		popPaneOverlayFn: func(id session.ClientID, pid session.PaneID) {
			poppedID = pid
			poppedCalled = true
		},
	}

	return m, c,
		func() (session.PaneID, modes.PaneMode) { return lastPushedID, lastPushedMode },
		func() (session.PaneID, bool) { return poppedID, poppedCalled }
}

// TestEnterCopyMode_AttachesOverlay verifies that EnterCopyMode registers a
// copy.Mode as the active pane overlay for the target pane.
func TestEnterCopyMode_AttachesOverlay(t *testing.T) {
	m, _, pushed, _ := newTestMutatorWithCopyMode()

	if err := m.EnterCopyMode("c1", false); err != nil {
		t.Fatalf("EnterCopyMode: %v", err)
	}

	paneID, mode := pushed()
	if mode == nil {
		t.Fatal("pushPaneOverlayFn not called or mode is nil")
	}
	if paneID != session.PaneID(1) {
		t.Errorf("pushed pane ID = %v, want 1", paneID)
	}
	if _, ok := mode.(*copymode.Mode); !ok {
		t.Errorf("pushed mode type = %T, want *copy.Mode", mode)
	}
}

// TestEnterCopyMode_ClientNotFound verifies an error is returned when the
// client does not exist.
func TestEnterCopyMode_ClientNotFound(t *testing.T) {
	m, _, _, _ := newTestMutatorWithCopyMode()

	if err := m.EnterCopyMode("no-such-client", false); err == nil {
		t.Fatal("expected error for unknown client, got nil")
	}
}

// TestEnterCopyMode_NoSession verifies an error is returned when the client
// has no attached session.
func TestEnterCopyMode_NoSession(t *testing.T) {
	m, client, _, _ := newTestMutatorWithCopyMode()
	client.Session = nil

	if err := m.EnterCopyMode("c1", false); err == nil {
		t.Fatal("expected error for client with no session, got nil")
	}
}

// ----- Server-level integration tests (key routing + paste buffer) -----------

// newSrvWithCopyMode builds a minimal srv suitable for testing pane mode key
// routing. It returns the srv, the clientConn, and the active paneID.
func newSrvWithCopyMode(t *testing.T) (s *srv, cc *clientConn, paneID session.PaneID) {
	t.Helper()
	state := session.NewServer()

	sess := session.NewSession(session.SessionID("s1"), "s1", nil)
	state.AddSession(sess)

	win := session.NewWindow(session.WindowID("w1"), "w1", nil)
	pid := session.PaneID(1)
	sp := newScrollablePane(pid, 10, 40, "hello world")
	win.AddPane(pid, sp)

	wl := &session.Winlink{Window: win, Session: sess, Index: 1}
	sess.Windows = append(sess.Windows, wl)
	sess.Current = wl

	client := session.NewClient(session.ClientID("c1"))
	state.Clients[client.ID] = client
	client.Session = sess

	cc = &clientConn{
		id:     session.ClientID("c1"),
		client: client,
		dirty:  make(chan struct{}, 1),
		paneOverlays: map[session.PaneID]modes.PaneMode{},
	}

	s = &srv{
		state: state,
		conns: map[session.ClientID]*clientConn{
			session.ClientID("c1"): cc,
		},
		done: make(chan struct{}),
	}

	return s, cc, pid
}

// TestPaneMode_QKeyPopsOverlay verifies that pressing 'q' in copy-mode removes
// the pane overlay.
func TestPaneMode_QKeyPopsOverlay(t *testing.T) {
	s, cc, paneID := newSrvWithCopyMode(t)

	// Attach a copy-mode overlay manually.
	sb := &stubScrollback{lines: makeTestLines("hello"), width: 40, height: 10}
	mode := copymode.New(sb)
	cc.paneOverlays[paneID] = mode

	// Send 'q' key.
	outcome := mode.Key(keys.Key{Code: keys.KeyCode('q')})
	if outcome.Kind != modes.KindCloseMode {
		t.Fatalf("'q' outcome = %v, want KindCloseMode", outcome.Kind)
	}

	// Simulate what the server loop does on KindCloseMode.
	s.popPaneOverlay(cc.id, paneID)

	// The overlay should be gone.
	s.mu.Lock()
	_, still := cc.paneOverlays[paneID]
	s.mu.Unlock()

	if still {
		t.Error("pane overlay still present after 'q' key")
	}
}

// TestPaneMode_CopySelectionStoresToPasteBuffer verifies that copying a
// selection stores the selected text in the server's paste buffer.
func TestPaneMode_CopySelectionStoresToPasteBuffer(t *testing.T) {
	s, cc, paneID := newSrvWithCopyMode(t)

	lines := makeTestLines("hello world")
	sb := &stubScrollback{lines: lines, width: 40, height: 10}
	mode := copymode.New(sb)
	cc.paneOverlays[paneID] = mode

	// Begin selection at column 0, then move right to select "hello".
	mode.Command("begin-selection") //nolint:errcheck
	for i := 0; i < 4; i++ {
		mode.Command("cursor-right") //nolint:errcheck
	}

	// copy-selection returns a Command outcome with CopyCommand.
	outcome := mode.Command("copy-selection")
	if outcome.Kind != modes.KindCommand {
		t.Fatalf("copy-selection outcome = %v, want KindCommand", outcome.Kind)
	}
	copyCmd, ok := outcome.Cmd.(copymode.CopyCommand)
	if !ok {
		t.Fatalf("outcome.Cmd type = %T, want CopyCommand", outcome.Cmd)
	}
	if copyCmd.Text == "" {
		t.Fatal("copied text is empty")
	}

	// Simulate the server loop: push to buffer then pop the overlay.
	s.mu.Lock()
	s.state.Buffers.Push("", []byte(copyCmd.Text))
	s.mu.Unlock()
	s.popPaneOverlay(cc.id, paneID)

	// Check paste buffer.
	buf := s.state.Buffers.Top()
	if buf == nil {
		t.Fatal("paste buffer is empty after copy")
	}
	if string(buf.Data) != copyCmd.Text {
		t.Errorf("buffer data = %q, want %q", string(buf.Data), copyCmd.Text)
	}

	// Overlay should be gone.
	s.mu.Lock()
	_, still := cc.paneOverlays[paneID]
	s.mu.Unlock()
	if still {
		t.Error("pane overlay still present after copy-selection")
	}
}

// ---- stub helpers used only in this test file --------------------------------

// stubScrollback is a local copy of the test helper from modes/copy tests,
// reused here to construct copy-mode instances without a real pane.
type stubScrollback struct {
	lines  []copymode.Line
	width  int
	height int
}

func (s *stubScrollback) Lines() []copymode.Line { return s.lines }
func (s *stubScrollback) Width() int             { return s.width }
func (s *stubScrollback) Height() int            { return s.height }

// makeTestLines builds a single-element []copymode.Line from a plain string.
func makeTestLines(text string) []copymode.Line {
	runes := []rune(text)
	line := make(copymode.Line, len(runes))
	for i, ch := range runes {
		line[i] = modes.Cell{Char: ch}
	}
	return []copymode.Line{line}
}

// Ensure scrollablePane satisfies session.Pane at compile time.
// The remaining methods are inherited from the embedded fakePane.
var _ session.Pane = (*scrollablePane)(nil)
