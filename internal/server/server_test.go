package server_test

import (
	"bytes"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/layout"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/server"
	"github.com/dhamidi/dmux/internal/session"
)

// pipeListener is a net.Listener backed by pre-connected net.Pipe() pairs.
// Call dial() to obtain the client-side connection; the server side is queued
// for the next Accept() call.
type pipeListener struct {
	ch     chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{
		ch:     make(chan net.Conn, 8),
		closed: make(chan struct{}),
	}
}

// dial creates a pipe pair, queues the server end, and returns the client end.
func (pl *pipeListener) dial() net.Conn {
	serverSide, clientSide := net.Pipe()
	pl.ch <- serverSide
	return clientSide
}

func (pl *pipeListener) Accept() (net.Conn, error) {
	select {
	case nc := <-pl.ch:
		return nc, nil
	case <-pl.closed:
		return nil, &net.OpError{Op: "accept", Err: net.ErrClosed}
	}
}

func (pl *pipeListener) Close() error {
	pl.once.Do(func() { close(pl.closed) })
	return nil
}

func (pl *pipeListener) Addr() net.Addr { return pipeAddr{} }

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }

// fakeSignal satisfies os.Signal for synthetic signal injection in tests.
type fakeSignal string

func (s fakeSignal) String() string { return string(s) }
func (s fakeSignal) Signal()        {}

var _ os.Signal = fakeSignal("")

// fixedClock returns a Clock that always returns t.
func fixedClock(t time.Time) server.Clock {
	return func() time.Time { return t }
}

// sendHandshake writes a complete VERSION + IDENTIFY sequence to conn.
// It uses version 1 and minimal identify fields sufficient for the server
// to complete the handshake.
func sendHandshake(t *testing.T, conn net.Conn) {
	t.Helper()

	if err := proto.WriteMsg(conn, proto.MsgVersion, (proto.VersionMsg{Version: 1}).Encode()); err != nil {
		t.Fatalf("sendHandshake: write VERSION: %v", err)
	}

	msgs := []struct {
		t proto.MsgType
		p []byte
	}{
		{proto.MsgIdentifyFlags, (proto.IdentifyFlagsMsg{Flags: 0}).Encode()},
		{proto.MsgIdentifyTerm, (proto.IdentifyTermMsg{Term: "xterm-256color"}).Encode()},
		{proto.MsgIdentifyTTYName, (proto.IdentifyTTYNameMsg{TTYName: "/dev/pts/0"}).Encode()},
		{proto.MsgIdentifyCWD, (proto.IdentifyCWDMsg{CWD: "/tmp"}).Encode()},
		{proto.MsgIdentifyEnviron, (proto.IdentifyEnvironMsg{Pairs: []string{"TERM=xterm-256color", "HOME=/home/user"}}).Encode()},
		{proto.MsgIdentifyClientPID, (proto.IdentifyClientPIDMsg{PID: 9999}).Encode()},
		{proto.MsgIdentifyFeatures, (proto.IdentifyFeaturesMsg{Features: 0}).Encode()},
		{proto.MsgIdentifyDone, (proto.IdentifyDoneMsg{}).Encode()},
	}
	for _, m := range msgs {
		if err := proto.WriteMsg(conn, m.t, m.p); err != nil {
			t.Fatalf("sendHandshake: write %s: %v", m.t, err)
		}
	}
}

// startServer launches server.Run in a goroutine and returns a channel that
// receives the Run() return value when the server exits.
func startServer(cfg server.Config) <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- server.Run(cfg) }()
	return ch
}

// TestHandshake verifies that the server accepts a connection and completes
// the VERSION + IDENTIFY handshake without error, then shuts down cleanly
// when a signal is received.
func TestHandshake(t *testing.T) {
	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
	})

	clientConn := pl.dial()
	defer clientConn.Close()

	sendHandshake(t, clientConn)

	// Give the server time to process the identify sequence.
	time.Sleep(10 * time.Millisecond)

	// Trigger graceful shutdown via signal.
	sigs <- fakeSignal("SIGTERM")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within timeout after SIGTERM")
	}
}

// TestRedrawOnStateChange verifies that a RESIZE message from a connected
// client marks that client dirty for redraw, observable via Config.OnDirty.
func TestRedrawOnStateChange(t *testing.T) {
	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	dirtyIDs := make(chan session.ClientID, 4)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
		OnDirty: func(id session.ClientID) {
			select {
			case dirtyIDs <- id:
			default:
			}
		},
	})

	clientConn := pl.dial()
	defer clientConn.Close()

	sendHandshake(t, clientConn)

	// Send a RESIZE to trigger the dirty mark.
	rm := proto.ResizeMsg{Width: 220, Height: 50}
	if err := proto.WriteMsg(clientConn, proto.MsgResize, rm.Encode()); err != nil {
		t.Fatalf("write RESIZE: %v", err)
	}

	// Expect OnDirty to fire with some client ID.
	select {
	case id := <-dirtyIDs:
		if id == "" {
			t.Fatal("OnDirty called with empty client ID")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnDirty was not called after RESIZE within timeout")
	}

	sigs <- fakeSignal("SIGHUP")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within timeout after SIGHUP")
	}
}

// TestShutdownOnSignal verifies that Run() returns cleanly when a shutdown
// signal is delivered on Config.Signals, even with no connected clients.
func TestShutdownOnSignal(t *testing.T) {
	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
	})

	sigs <- fakeSignal("SIGTERM")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within timeout after SIGTERM")
	}
}

// TestVersionMismatch verifies that the server rejects clients that send an
// incompatible protocol version and sends them an EXIT message.
func TestVersionMismatch(t *testing.T) {
	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
	})

	clientConn := pl.dial()
	defer clientConn.Close()

	// Send an incompatible version number.
	badVM := proto.VersionMsg{Version: 99}
	if err := proto.WriteMsg(clientConn, proto.MsgVersion, badVM.Encode()); err != nil {
		t.Fatalf("write bad VERSION: %v", err)
	}

	// The server should respond with EXIT.
	msgType, payload, err := proto.ReadMsg(clientConn)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if msgType != proto.MsgExit {
		t.Fatalf("expected MsgExit, got %s", msgType)
	}
	var em proto.ExitMsg
	if err := em.Decode(payload); err != nil {
		t.Fatalf("decode EXIT: %v", err)
	}
	if em.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", em.Code)
	}

	sigs <- fakeSignal("SIGTERM")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within timeout")
	}
}

// TestCommandDispatch verifies that a MsgCommand from a connected client is
// dispatched through the command system and the output is returned as MsgStdout.
// list-commands is used because it always produces output regardless of server
// state, exercising the full dispatch→encode→send pipeline.
func TestCommandDispatch(t *testing.T) {
	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
	})

	clientConn := pl.dial()
	defer clientConn.Close()

	sendHandshake(t, clientConn)

	// Give the server time to enter clientLoop.
	time.Sleep(10 * time.Millisecond)

	// Send list-commands: this command lists all registered commands and always
	// produces non-empty output, proving the dispatch and output paths work.
	cm := proto.CommandMsg{Argv: []string{"list-commands"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write MsgCommand: %v", err)
	}

	// Expect a MsgStdout response containing the session-related command names.
	if err := clientConn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	msgType, payload, err := proto.ReadMsg(clientConn)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if msgType != proto.MsgStdout {
		t.Fatalf("expected MsgStdout, got %s", msgType)
	}
	var sm proto.StdoutMsg
	if err := sm.Decode(payload); err != nil {
		t.Fatalf("decode StdoutMsg: %v", err)
	}
	if !strings.Contains(string(sm.Data), "list-sessions") {
		t.Fatalf("expected output to contain 'list-sessions', got: %q", sm.Data)
	}
	// Remove deadline before shutdown.
	clientConn.SetDeadline(time.Time{}) //nolint:errcheck

	sigs <- fakeSignal("SIGTERM")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within timeout after SIGTERM")
	}
}

// TestAutoCreateInitialSession verifies that the server automatically creates
// a session when the first client attaches to a server with no sessions (S6).
func TestAutoCreateInitialSession(t *testing.T) {
	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
	})

	clientConn := pl.dial()
	defer clientConn.Close()

	sendHandshake(t, clientConn)

	// Give the server time to process the identify sequence and auto-create a session.
	time.Sleep(20 * time.Millisecond)

	// Verify via list-sessions that exactly one session was created.
	cm := proto.CommandMsg{Argv: []string{"list-sessions"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write list-sessions: %v", err)
	}
	if err := clientConn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	msgType, payload, err := proto.ReadMsg(clientConn)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if msgType != proto.MsgStdout {
		t.Fatalf("expected MsgStdout, got %s", msgType)
	}
	var sm proto.StdoutMsg
	if err := sm.Decode(payload); err != nil {
		t.Fatalf("decode StdoutMsg: %v", err)
	}
	// The auto-created session is named "session1".
	if !strings.Contains(string(sm.Data), "session1:") {
		t.Fatalf("expected auto-created session 'session1' in list-sessions output, got: %q", sm.Data)
	}
	clientConn.SetDeadline(time.Time{}) //nolint:errcheck

	sigs <- fakeSignal("SIGTERM")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within timeout after SIGTERM")
	}
}

// TestMsgStdinKeyBinding verifies that a bound key received via MsgStdin is
// dispatched through the key table and triggers the corresponding command.
// It binds 'q' in the root table to new-session, sends 'q' as stdin input,
// and then confirms a new session was created by listing sessions.
func TestMsgStdinKeyBinding(t *testing.T) {
	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	dirtyIDs := make(chan session.ClientID, 8)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
		OnDirty: func(id session.ClientID) {
			select {
			case dirtyIDs <- id:
			default:
			}
		},
	})

	clientConn := pl.dial()
	defer clientConn.Close()

	sendHandshake(t, clientConn)
	// Wait for the identify sequence to complete and the auto-session to be created.
	time.Sleep(20 * time.Millisecond)

	// Bind 'q' in the root table to new-session.
	bindCmd := proto.CommandMsg{Argv: []string{"bind-key", "-T", "root", "q", "new-session"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, bindCmd.Encode()); err != nil {
		t.Fatalf("write bind-key: %v", err)
	}

	// Send MsgStdin with 'q' (0x71) to trigger the bound command.
	stdinMsg := proto.StdinMsg{Data: []byte("q")}
	if err := proto.WriteMsg(clientConn, proto.MsgStdin, stdinMsg.Encode()); err != nil {
		t.Fatalf("write MsgStdin: %v", err)
	}

	// Wait for OnDirty to fire (signals the MsgStdin was processed).
	select {
	case <-dirtyIDs:
	case <-time.After(2 * time.Second):
		t.Fatal("OnDirty was not called after MsgStdin within timeout")
	}

	// Confirm the bound command ran by verifying a second session was created.
	listCmd := proto.CommandMsg{Argv: []string{"list-sessions"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, listCmd.Encode()); err != nil {
		t.Fatalf("write list-sessions: %v", err)
	}
	if err := clientConn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	msgType, payload, err := proto.ReadMsg(clientConn)
	if err != nil {
		t.Fatalf("read list-sessions response: %v", err)
	}
	if msgType != proto.MsgStdout {
		t.Fatalf("expected MsgStdout, got %s", msgType)
	}
	var outMsg proto.StdoutMsg
	if err := outMsg.Decode(payload); err != nil {
		t.Fatalf("decode StdoutMsg: %v", err)
	}
	// The auto-created session is "session1"; new-session creates "session2".
	if !strings.Contains(string(outMsg.Data), "session2:") {
		t.Fatalf("expected session 'session2' in list-sessions output, got: %q", outMsg.Data)
	}
	clientConn.SetDeadline(time.Time{}) //nolint:errcheck

	sigs <- fakeSignal("SIGTERM")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within timeout after SIGTERM")
	}
}

// testFakePane is a minimal session.Pane for use in TestRenderLoop.
// Snapshot returns a fixed 3×2 grid with distinct characters.
type testFakePane struct{}

func (f *testFakePane) Title() string                               { return "" }
func (f *testFakePane) Resize(cols, rows int) error                 { return nil }
func (f *testFakePane) Close() error                                { return nil }
func (f *testFakePane) CaptureContent(history bool) ([]byte, error) { return nil, nil }
func (f *testFakePane) Respawn(shell string) error                  { return nil }
func (f *testFakePane) SendKey(key keys.Key) error                  { return nil }
func (f *testFakePane) Write(data []byte) error                     { return nil }
func (f *testFakePane) Snapshot() pane.CellGrid {
	return pane.CellGrid{
		Rows: 2, Cols: 3,
		Cells: []pane.Cell{
			{Char: 'A'}, {Char: 'B'}, {Char: 'C'},
			{Char: 'D'}, {Char: 'E'}, {Char: 'F'},
		},
	}
}

// TestRenderLoop verifies that after a client attaches to a session containing
// a pane, the render goroutine sends a MsgStdout whose payload begins with the
// ANSI cursor-home + erase-display sequence ("\x1b[H\x1b[2J").
func TestRenderLoop(t *testing.T) {
	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	// Build initial server state: one session with a window and a fake pane.
	state := session.NewServer()
	sess := session.NewSession(session.SessionID("$1"), "test-sess", state.Options)
	paneID := session.PaneID(1)
	win := session.NewWindow(session.WindowID("@1"), "main", sess.Options)
	win.AddPane(paneID, &testFakePane{})
	win.Layout = layout.New(3, 2, paneID)
	sess.AddWindow(win)
	state.AddSession(sess)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
		State:    state,
	})

	clientConn := pl.dial()
	defer clientConn.Close()

	sendHandshake(t, clientConn)

	// Give the server time to enter clientLoop.
	time.Sleep(10 * time.Millisecond)

	// Attach the client to the test session. attach-session produces no
	// command output, but markDirty is called afterwards, which triggers
	// renderLoop to compose and send the frame.
	cm := proto.CommandMsg{Argv: []string{"attach-session", "-t", "test-sess"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write attach-session: %v", err)
	}

	// Read the MsgStdout emitted by renderLoop.
	if err := clientConn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	msgType, payload, err := proto.ReadMsg(clientConn)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if msgType != proto.MsgStdout {
		t.Fatalf("expected MsgStdout, got %s", msgType)
	}
	var sm proto.StdoutMsg
	if err := sm.Decode(payload); err != nil {
		t.Fatalf("decode StdoutMsg: %v", err)
	}
	if !bytes.HasPrefix(sm.Data, []byte("\x1b[H\x1b[2J")) {
		t.Fatalf("expected ANSI clear/home prefix, got: %q", sm.Data)
	}
	clientConn.SetDeadline(time.Time{}) //nolint:errcheck

	sigs <- fakeSignal("SIGTERM")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within timeout after SIGTERM")
	}
}

// TestClientShutdownRequest verifies that a client sending MsgShutdown
// triggers server shutdown without an external signal.
func TestClientShutdownRequest(t *testing.T) {
	pl := newPipeListener()

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Now:      fixedClock(time.Time{}),
		// No Signals: shutdown must come from the client.
	})

	clientConn := pl.dial()
	defer clientConn.Close()

	sendHandshake(t, clientConn)

	// Give the server time to enter clientLoop.
	time.Sleep(10 * time.Millisecond)

	// Request shutdown from the client side.
	if err := proto.WriteMsg(clientConn, proto.MsgShutdown, (proto.ShutdownMsg{}).Encode()); err != nil {
		t.Fatalf("write SHUTDOWN: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within timeout after MsgShutdown")
	}
}
