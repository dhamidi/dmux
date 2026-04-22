package server_test

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/layout"
	"github.com/dhamidi/dmux/internal/modes"
	"github.com/dhamidi/dmux/internal/options"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/server"
	"github.com/dhamidi/dmux/internal/session"
)

// advancingClock returns a Clock that starts at start and advances by step
// each call.
func advancingClock(start time.Time, step time.Duration) server.Clock {
	t := start
	var mu sync.Mutex
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		now := t
		t = t.Add(step)
		return now
	}
}

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

// readStdoutUntilContains reads MsgStdout messages from conn until one contains
// needle, then returns its payload. It fails the test if the deadline elapses
// before a matching message is found. A fresh deadline must be set by the caller
// before invoking this helper.
func readStdoutUntilContains(t *testing.T, conn net.Conn, needle string) string {
	t.Helper()
	for {
		msgType, payload, err := proto.ReadMsg(conn)
		if err != nil {
			t.Fatalf("readStdoutUntilContains: read: %v", err)
		}
		if msgType != proto.MsgStdout {
			t.Fatalf("readStdoutUntilContains: expected MsgStdout, got %s", msgType)
		}
		var sm proto.StdoutMsg
		if err := sm.Decode(payload); err != nil {
			t.Fatalf("readStdoutUntilContains: decode: %v", err)
		}
		if strings.Contains(string(sm.Data), needle) {
			return string(sm.Data)
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

	// Read MsgStdout messages until one contains command output. The initial
	// render may arrive first now that a window is auto-created on connect.
	if err := clientConn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	readStdoutUntilContains(t, clientConn, "list-sessions")
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
	// The auto-created session is named "session1". Read until we find it in
	// command output; earlier messages may be render frames.
	readStdoutUntilContains(t, clientConn, "session1:")
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
	// The auto-created session is "session1"; new-session creates "session2".
	// Read until command output arrives; render frames may appear first.
	readStdoutUntilContains(t, clientConn, "session2:")
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

// TestPrefixKeyNewWindow verifies that the default Ctrl+b, c prefix sequence
// creates a new window via the switch-key-table → prefix → new-window flow.
func TestPrefixKeyNewWindow(t *testing.T) {
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
	time.Sleep(20 * time.Millisecond)

	// Send Ctrl+b (0x02) to switch to the prefix table.
	stdinMsg := proto.StdinMsg{Data: []byte{0x02}}
	if err := proto.WriteMsg(clientConn, proto.MsgStdin, stdinMsg.Encode()); err != nil {
		t.Fatalf("write Ctrl+b: %v", err)
	}
	select {
	case <-dirtyIDs:
	case <-time.After(2 * time.Second):
		t.Fatal("OnDirty not called after Ctrl+b")
	}

	// Send 'c' to trigger new-window in the prefix table.
	stdinMsg = proto.StdinMsg{Data: []byte("c")}
	if err := proto.WriteMsg(clientConn, proto.MsgStdin, stdinMsg.Encode()); err != nil {
		t.Fatalf("write c: %v", err)
	}
	select {
	case <-dirtyIDs:
	case <-time.After(2 * time.Second):
		t.Fatal("OnDirty not called after c")
	}

	// Verify the new window was created by listing windows.
	listCmd := proto.CommandMsg{Argv: []string{"list-windows"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, listCmd.Encode()); err != nil {
		t.Fatalf("write list-windows: %v", err)
	}
	if err := clientConn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	// The auto-created window is index 0; new-window creates index 1.
	readStdoutUntilContains(t, clientConn, "1:")
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

// TestConfigFileAutoLoad verifies that when ConfigFile is set, the server
// sources the file at startup and executes the commands in it.
func TestConfigFileAutoLoad(t *testing.T) {
	// Write a config file containing a single new-session command.
	f, err := os.CreateTemp("", "dmux-config-*.conf")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString("new-session -s test\n"); err != nil {
		t.Fatalf("write config: %v", err)
	}
	f.Close()

	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	done := startServer(server.Config{
		Listener:   pl,
		Log:        io.Discard,
		Signals:    sigs,
		Now:        fixedClock(time.Time{}),
		ConfigFile: f.Name(),
	})

	clientConn := pl.dial()
	defer clientConn.Close()

	sendHandshake(t, clientConn)

	// Give the server time to process the identify sequence.
	time.Sleep(20 * time.Millisecond)

	// List sessions and verify the "test" session from the config file exists.
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
	if !strings.Contains(string(sm.Data), "test:") {
		t.Fatalf("expected session 'test' in list-sessions output, got: %q", sm.Data)
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

// testFakePaneWithPID is like testFakePane but returns a configurable ShellPID.
type testFakePaneWithPID struct {
	pid int
}

func (f *testFakePaneWithPID) Title() string                               { return "" }
func (f *testFakePaneWithPID) Resize(cols, rows int) error                 { return nil }
func (f *testFakePaneWithPID) Close() error                                { return nil }
func (f *testFakePaneWithPID) CaptureContent(history bool) ([]byte, error) { return nil, nil }
func (f *testFakePaneWithPID) Respawn(shell string) error                  { return nil }
func (f *testFakePaneWithPID) SendKey(key keys.Key) error                  { return nil }
func (f *testFakePaneWithPID) Write(data []byte) error                     { return nil }
func (f *testFakePaneWithPID) ShellPID() int                               { return f.pid }
func (f *testFakePaneWithPID) LastOutputAt() time.Time                     { return time.Time{} }
func (f *testFakePaneWithPID) ConsumeBell() bool                           { return false }
func (f *testFakePaneWithPID) ClearHistory()                               {}
func (f *testFakePaneWithPID) ClearScreen() error                          { return nil }
func (f *testFakePaneWithPID) Snapshot() pane.CellGrid {
	return pane.CellGrid{Rows: 1, Cols: 1, Cells: []pane.Cell{{Char: 'X'}}}
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
func (f *testFakePane) ShellPID() int                               { return 0 }
func (f *testFakePane) LastOutputAt() time.Time                     { return time.Time{} }
func (f *testFakePane) ConsumeBell() bool                           { return false }
func (f *testFakePane) ClearHistory()                               {}
func (f *testFakePane) ClearScreen() error                          { return nil }
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

// TestRenderLoopStatusBar verifies that when the "status" option is "on",
// the render loop includes a status bar row in the composed output. The
// session name appears in the status bar via the #{session_name} variable
// in the default status-right format string.
func TestRenderLoopStatusBar(t *testing.T) {
	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	// Build a session with a fake pane; status is "on" by default.
	state := session.NewServer()
	sess := session.NewSession(session.SessionID("$1"), "statustest", state.Options)
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
	time.Sleep(10 * time.Millisecond)

	// Attach the client to the status-test session to trigger a render.
	cm := proto.CommandMsg{Argv: []string{"attach-session", "-t", "statustest"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write attach-session: %v", err)
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

	// The default status-right is " #{session_name}" which expands to
	// " statustest". Verify the session name appears in the status bar row.
	if !strings.Contains(string(sm.Data), "statustest") {
		preview := sm.Data
		if len(preview) > 200 {
			preview = preview[:200]
		}
		t.Fatalf("expected 'statustest' in ANSI output (status bar), got: %q", preview)
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

// TestAutoRename verifies that when the automatic-rename window option is on
// and the active pane has a non-zero ShellPID, the tick loop updates the
// window name to the value returned by ForegroundCommand.
func TestAutoRename(t *testing.T) {
	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	const fakePID = 12345
	const expectedName = "vim"

	// Build initial server state: one session with a window and a pane that
	// has a non-zero ShellPID.
	state := session.NewServer()
	sess := session.NewSession(session.SessionID("$1"), "test-sess", state.Options)
	paneID := session.PaneID(1)
	win := session.NewWindow(session.WindowID("@1"), "bash", sess.Options)
	win.AddPane(paneID, &testFakePaneWithPID{pid: fakePID})
	win.Layout = layout.New(80, 24, paneID)
	sess.AddWindow(win)
	state.AddSession(sess)

	dirtyIDs := make(chan session.ClientID, 8)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
		State:    state,
		ForegroundCommand: func(pid int) string {
			if pid == fakePID {
				return expectedName
			}
			return ""
		},
		OnDirty: func(id session.ClientID) {
			select {
			case dirtyIDs <- id:
			default:
			}
		},
	})

	// Wait for the tick loop to fire (ticker fires every second).
	// We allow up to 3 seconds for the rename to happen.
	deadline := time.Now().Add(3 * time.Second)
	renamed := false
	for time.Now().Before(deadline) {
		// Check the window name directly on the shared state.
		// The server lock is internal, so we poll briefly.
		time.Sleep(50 * time.Millisecond)
		if win.Name == expectedName {
			renamed = true
			break
		}
	}

	sigs <- fakeSignal("SIGTERM")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within timeout after SIGTERM")
	}

	if !renamed {
		t.Fatalf("expected window name %q after auto-rename tick, got %q", expectedName, win.Name)
	}
}

// testCapturingPane is a session.Pane that records bytes written to it via Write.
type testCapturingPane struct {
	mu  sync.Mutex
	buf []byte
}

func (p *testCapturingPane) Written() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]byte, len(p.buf))
	copy(out, p.buf)
	return out
}

func (p *testCapturingPane) Title() string                               { return "" }
func (p *testCapturingPane) Resize(cols, rows int) error                 { return nil }
func (p *testCapturingPane) Close() error                                { return nil }
func (p *testCapturingPane) CaptureContent(history bool) ([]byte, error) { return nil, nil }
func (p *testCapturingPane) Respawn(shell string) error                  { return nil }
func (p *testCapturingPane) SendKey(key keys.Key) error                  { return nil }
func (p *testCapturingPane) LastOutputAt() time.Time                     { return time.Time{} }
func (p *testCapturingPane) ConsumeBell() bool                           { return false }
func (p *testCapturingPane) ClearHistory()                               {}
func (p *testCapturingPane) ClearScreen() error                          { return nil }
func (p *testCapturingPane) Write(data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.buf = append(p.buf, data...)
	return nil
}
func (p *testCapturingPane) ShellPID() int { return 0 }
func (p *testCapturingPane) Snapshot() pane.CellGrid {
	return pane.CellGrid{Rows: 1, Cols: 1, Cells: []pane.Cell{{Char: 'X'}}}
}

// buildSyncTestState creates a server state with one session, one window, and
// two capturing panes. If syncOn is true, synchronize-panes is enabled on the
// window so that input to the active pane is fanned out to the other pane.
func buildSyncTestState(t *testing.T, syncOn bool) (
	state *session.Server,
	active *testCapturingPane,
	other *testCapturingPane,
) {
	t.Helper()
	state = session.NewServer()
	// Pre-register synchronize-panes so we can set it before Run() is called.
	// loadDefaultOptions (called inside Run) will see it already registered
	// (idempotent) and reset the root to false; our window-local override wins.
	state.Options.Register("synchronize-panes", options.Bool, false)

	sess := session.NewSession(session.SessionID("$1"), "sync-sess", state.Options)
	paneID1 := session.PaneID(1)
	paneID2 := session.PaneID(2)
	active = &testCapturingPane{}
	other = &testCapturingPane{}

	win := session.NewWindow(session.WindowID("@1"), "main", sess.Options)
	win.AddPane(paneID1, active) // paneID1 becomes Active
	win.AddPane(paneID2, other)
	win.Layout = layout.New(80, 24, paneID1)

	if syncOn {
		if err := win.Options.Set("synchronize-panes", true); err != nil {
			t.Fatalf("set synchronize-panes: %v", err)
		}
	}

	sess.AddWindow(win)
	state.AddSession(sess)
	return state, active, other
}

// TestSynchronizePanes verifies that when synchronize-panes is on, every
// keystroke written to the active pane is also forwarded to all other panes in
// the window, and that when it is off only the active pane receives input.
func TestSynchronizePanes(t *testing.T) {
	runCase := func(t *testing.T, syncOn bool) (*testCapturingPane, *testCapturingPane) {
		t.Helper()
		state, active, other := buildSyncTestState(t, syncOn)

		pl := newPipeListener()
		sigs := make(chan os.Signal, 1)
		dirtyIDs := make(chan session.ClientID, 16)

		done := startServer(server.Config{
			Listener: pl,
			Log:      io.Discard,
			Signals:  sigs,
			Now:      fixedClock(time.Time{}),
			State:    state,
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
		time.Sleep(20 * time.Millisecond)

		// Drain all server output so renderLoop goroutine never blocks.
		go func() {
			for {
				if _, _, err := proto.ReadMsg(clientConn); err != nil {
					return
				}
			}
		}()

		// Attach the client to the pre-configured session.
		cm := proto.CommandMsg{Argv: []string{"attach-session", "-t", "sync-sess"}}
		if err := proto.WriteMsg(clientConn, proto.MsgCommand, cm.Encode()); err != nil {
			t.Fatalf("write attach-session: %v", err)
		}
		// Wait for the attach dirty event before proceeding.
		select {
		case <-dirtyIDs:
		case <-time.After(2 * time.Second):
			t.Fatal("OnDirty not called after attach-session")
		}

		// Send a raw keystroke.
		input := []byte("x")
		stdinMsg := proto.StdinMsg{Data: input}
		if err := proto.WriteMsg(clientConn, proto.MsgStdin, stdinMsg.Encode()); err != nil {
			t.Fatalf("write MsgStdin: %v", err)
		}
		// Wait for the dirty event that marks MsgStdin processing complete.
		select {
		case <-dirtyIDs:
		case <-time.After(2 * time.Second):
			t.Fatal("OnDirty not called after MsgStdin")
		}

		sigs <- fakeSignal("SIGTERM")
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Run() returned error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Run() did not return within timeout after SIGTERM")
		}

		return active, other
	}

	t.Run("on_both_panes_receive_input", func(t *testing.T) {
		active, other := runCase(t, true)
		if !bytes.Equal(active.Written(), []byte("x")) {
			t.Errorf("active pane: want %q, got %q", "x", active.Written())
		}
		if !bytes.Equal(other.Written(), []byte("x")) {
			t.Errorf("non-active pane: want %q with synchronize-panes on, got %q", "x", other.Written())
		}
	})

	t.Run("off_only_active_pane_receives_input", func(t *testing.T) {
		active, other := runCase(t, false)
		if !bytes.Equal(active.Written(), []byte("x")) {
			t.Errorf("active pane: want %q, got %q", "x", active.Written())
		}
		if len(other.Written()) != 0 {
			t.Errorf("non-active pane: want no input with synchronize-panes off, got %q", other.Written())
		}
	})
}

// buildMouseTestState creates a server state with mouse=on (session level),
// one session, and one window with two capturing panes laid out side by side.
// Pane 1 occupies cols 0-39, pane 2 occupies cols 40-79 (both full height).
func buildMouseTestState(t *testing.T, mouseOn bool) (
	state *session.Server,
	pane1 *testCapturingPane,
	pane2 *testCapturingPane,
	sess *session.Session,
	win *session.Window,
) {
	t.Helper()
	state = session.NewServer()
	state.Options.Register("mouse", options.Bool, false)

	sess = session.NewSession(session.SessionID("$1"), "mouse-sess", state.Options)
	if mouseOn {
		if err := sess.Options.Set("mouse", true); err != nil {
			t.Fatalf("set mouse option: %v", err)
		}
	}

	paneID1 := session.PaneID(1)
	paneID2 := session.PaneID(2)
	pane1 = &testCapturingPane{}
	pane2 = &testCapturingPane{}

	win = session.NewWindow(session.WindowID("@1"), "main", sess.Options)
	win.AddPane(paneID1, pane1) // paneID1 becomes Active
	win.AddPane(paneID2, pane2)

	// 80x24 layout split horizontally: pane1 left (0-39), pane2 right (40-79).
	win.Layout = layout.New(80, 24, paneID1)
	win.Layout.Split(paneID1, layout.Horizontal)

	sess.AddWindow(win)
	state.AddSession(sess)
	return state, pane1, pane2, sess, win
}

// encodeSGRMouse returns the raw SGR mouse escape sequence for a press event
// at the given 0-based column and row with the specified button code.
// button 0 = left, 64 = wheel-up, 65 = wheel-down.
func encodeSGRMouse(button, col, row int) []byte {
	// SGR format: ESC [ < btn ; col+1 ; row+1 M  (1-based coords)
	return []byte(fmt.Sprintf("\x1b[<%d;%d;%dM", button, col+1, row+1))
}

// TestMouseClickToFocus verifies that a left-click inside a non-active pane's
// rectangle (with mouse=on) changes the active pane to that pane.
func TestMouseClickToFocus(t *testing.T) {
	state, _, _, _, win := buildMouseTestState(t, true)

	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)
	dirtyIDs := make(chan session.ClientID, 16)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
		State:    state,
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
	time.Sleep(20 * time.Millisecond)

	// Drain server output so renderLoop never blocks.
	go func() {
		for {
			if _, _, err := proto.ReadMsg(clientConn); err != nil {
				return
			}
		}
	}()

	// Attach client to the pre-configured session.
	cm := proto.CommandMsg{Argv: []string{"attach-session", "-t", "mouse-sess"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write attach-session: %v", err)
	}
	select {
	case <-dirtyIDs:
	case <-time.After(2 * time.Second):
		t.Fatal("OnDirty not called after attach-session")
	}

	// Pane 1 is active (left half, cols 0-39). Click at col=50, row=10
	// which is inside pane 2 (right half, cols 40-79).
	clickBytes := encodeSGRMouse(0, 50, 10)
	stdinMsg := proto.StdinMsg{Data: clickBytes}
	if err := proto.WriteMsg(clientConn, proto.MsgStdin, stdinMsg.Encode()); err != nil {
		t.Fatalf("write MsgStdin (click): %v", err)
	}

	// Wait for the dirty event that marks MsgStdin processing complete.
	select {
	case <-dirtyIDs:
	case <-time.After(2 * time.Second):
		t.Fatal("OnDirty not called after mouse click")
	}

	// Verify the active pane changed to pane 2.
	if win.Active != session.PaneID(2) {
		t.Errorf("expected active pane 2, got %v", win.Active)
	}

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

// TestMouseOffPassthrough verifies that when mouse=off, mouse events are
// forwarded as raw bytes to the active pane without routing.
func TestMouseOffPassthrough(t *testing.T) {
	state, activePaneCapture, _, _, _ := buildMouseTestState(t, false)

	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)
	dirtyIDs := make(chan session.ClientID, 16)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
		State:    state,
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
	time.Sleep(20 * time.Millisecond)

	go func() {
		for {
			if _, _, err := proto.ReadMsg(clientConn); err != nil {
				return
			}
		}
	}()

	cm := proto.CommandMsg{Argv: []string{"attach-session", "-t", "mouse-sess"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write attach-session: %v", err)
	}
	select {
	case <-dirtyIDs:
	case <-time.After(2 * time.Second):
		t.Fatal("OnDirty not called after attach-session")
	}

	// Send a mouse click event.
	clickBytes := encodeSGRMouse(0, 50, 10)
	stdinMsg := proto.StdinMsg{Data: clickBytes}
	if err := proto.WriteMsg(clientConn, proto.MsgStdin, stdinMsg.Encode()); err != nil {
		t.Fatalf("write MsgStdin (mouse): %v", err)
	}

	select {
	case <-dirtyIDs:
	case <-time.After(2 * time.Second):
		t.Fatal("OnDirty not called after MsgStdin")
	}

	// The raw bytes must have been forwarded to the active pane.
	written := activePaneCapture.Written()
	if !bytes.Equal(written, clickBytes) {
		t.Errorf("expected raw bytes %q forwarded to active pane, got %q", clickBytes, written)
	}

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

// TestMouseWheelScroll verifies that wheel-up events (mouse=on) are written
// as raw bytes to the active pane.
func TestMouseWheelScroll(t *testing.T) {
	state, activePaneCapture, _, _, _ := buildMouseTestState(t, true)

	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)
	dirtyIDs := make(chan session.ClientID, 16)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
		State:    state,
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
	time.Sleep(20 * time.Millisecond)

	go func() {
		for {
			if _, _, err := proto.ReadMsg(clientConn); err != nil {
				return
			}
		}
	}()

	cm := proto.CommandMsg{Argv: []string{"attach-session", "-t", "mouse-sess"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write attach-session: %v", err)
	}
	select {
	case <-dirtyIDs:
	case <-time.After(2 * time.Second):
		t.Fatal("OnDirty not called after attach-session")
	}

	// Wheel-up: SGR button 64.
	wheelBytes := encodeSGRMouse(64, 10, 5)
	stdinMsg := proto.StdinMsg{Data: wheelBytes}
	if err := proto.WriteMsg(clientConn, proto.MsgStdin, stdinMsg.Encode()); err != nil {
		t.Fatalf("write MsgStdin (wheel): %v", err)
	}

	select {
	case <-dirtyIDs:
	case <-time.After(2 * time.Second):
		t.Fatal("OnDirty not called after MsgStdin")
	}

	// The raw wheel bytes must have been written to the active pane.
	written := activePaneCapture.Written()
	if !bytes.Equal(written, wheelBytes) {
		t.Errorf("expected wheel bytes %q written to active pane, got %q", wheelBytes, written)
	}

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

// TestHookAfterNewSession verifies that a hook registered for after-new-session
// fires when new-session is called.
func TestHookAfterNewSession(t *testing.T) {
	// Pre-register a hook on the shared state that signals a channel.
	state := session.NewServer()
	hookFired := make(chan struct{}, 1)
	state.Hooks.Register("after-new-session", "test", func() {
		select {
		case hookFired <- struct{}{}:
		default:
		}
	})

	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

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

	// Give the server time to process the identify sequence and auto-create
	// an initial session (which will fire the hook once).
	time.Sleep(20 * time.Millisecond)
	// Drain the auto-created session hook.
	select {
	case <-hookFired:
	case <-time.After(2 * time.Second):
		t.Fatal("hook for after-new-session did not fire for auto-created session")
	}

	// Now explicitly create another session and verify the hook fires again.
	cm := proto.CommandMsg{Argv: []string{"new-session", "-s", "hooktest"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write new-session command: %v", err)
	}

	select {
	case <-hookFired:
		// Hook fired for the explicitly created session.
	case <-time.After(2 * time.Second):
		t.Fatal("hook for after-new-session did not fire after new-session command")
	}

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

// testMonitorPane is a fake pane with configurable LastOutputAt and ConsumeBell
// for testing the monitor-activity, monitor-silence, and monitor-bell features.
type testMonitorPane struct {
	lastOutputAt time.Time
	bellResult   bool
	mu           sync.Mutex
}

func (p *testMonitorPane) Title() string                               { return "" }
func (p *testMonitorPane) Resize(cols, rows int) error                 { return nil }
func (p *testMonitorPane) Close() error                                { return nil }
func (p *testMonitorPane) CaptureContent(history bool) ([]byte, error) { return nil, nil }
func (p *testMonitorPane) Respawn(shell string) error                  { return nil }
func (p *testMonitorPane) SendKey(key keys.Key) error                  { return nil }
func (p *testMonitorPane) Write(data []byte) error                     { return nil }
func (p *testMonitorPane) ShellPID() int                               { return 0 }
func (p *testMonitorPane) Snapshot() pane.CellGrid {
	return pane.CellGrid{Rows: 1, Cols: 1, Cells: []pane.Cell{{Char: 'X'}}}
}
func (p *testMonitorPane) LastOutputAt() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastOutputAt
}
func (p *testMonitorPane) ConsumeBell() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	v := p.bellResult
	p.bellResult = false
	return v
}
func (p *testMonitorPane) ClearHistory()       {}
func (p *testMonitorPane) ClearScreen() error  { return nil }

// buildMonitorTestState creates a server state with one session, one window,
// and one testMonitorPane. The monitor options are pre-registered so that
// tests can call win.Options.Set before Run is called.
func buildMonitorTestState(t *testing.T, p *testMonitorPane) (
	state *session.Server,
	sess *session.Session,
	win *session.Window,
) {
	t.Helper()
	state = session.NewServer()
	// Pre-register monitor options so win.Options.Set works before Run().
	state.Options.Register("monitor-activity", options.Bool, false)
	state.Options.Register("monitor-bell", options.Bool, false)
	state.Options.Register("monitor-silence", options.Int, 0)

	sess = session.NewSession(session.SessionID("$1"), "monitor-sess", state.Options)
	paneID := session.PaneID(1)

	win = session.NewWindow(session.WindowID("@1"), "main", sess.Options)
	win.AddPane(paneID, p)
	win.Layout = layout.New(80, 24, paneID)

	sess.AddWindow(win)
	state.AddSession(sess)
	return state, sess, win
}

// drainClientConn reads all messages from conn and signals bellCh when a BEL
// character is detected in a payload. It stops when the connection is closed.
func drainClientConn(conn net.Conn, bellCh chan<- struct{}) {
	for {
		_, payload, err := proto.ReadMsg(conn)
		if err != nil {
			return
		}
		if bytes.IndexByte(payload, '\x07') >= 0 {
			select {
			case bellCh <- struct{}{}:
			default:
			}
		}
	}
}

// TestMonitorActivity verifies that when monitor-activity is on and a pane
// has output after LastMonitorCheck, the window's ActivityFlag is set and a
// BEL is sent to the client.
func TestMonitorActivity(t *testing.T) {
	epoch := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	outputTime := epoch.Add(time.Hour)

	fp := &testMonitorPane{lastOutputAt: outputTime}
	state, _, win := buildMonitorTestState(t, fp)

	if err := win.Options.Set("monitor-activity", true); err != nil {
		t.Fatalf("set monitor-activity: %v", err)
	}

	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(epoch.Add(2 * time.Hour)),
		State:    state,
	})

	clientConn := pl.dial()
	defer clientConn.Close()
	sendHandshake(t, clientConn)
	time.Sleep(20 * time.Millisecond)

	bellCh := make(chan struct{}, 4)
	go drainClientConn(clientConn, bellCh)

	cm := proto.CommandMsg{Argv: []string{"attach-session", "-t", "monitor-sess"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write attach-session: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		if win.ActivityFlag {
			break
		}
	}

	sigs <- fakeSignal("SIGTERM")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within timeout after SIGTERM")
	}

	if !win.ActivityFlag {
		t.Fatal("expected ActivityFlag to be set after monitor-activity sweep")
	}
	select {
	case <-bellCh:
	default:
		t.Fatal("expected BEL to be sent to client after monitor-activity")
	}
}

// TestMonitorSilence verifies that when monitor-silence is set and a pane has
// been silent for longer than the configured duration, a BEL is sent.
func TestMonitorSilence(t *testing.T) {
	epoch := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	lastOut := epoch.Add(-10 * time.Second)

	fp := &testMonitorPane{lastOutputAt: lastOut}
	state, _, win := buildMonitorTestState(t, fp)

	if err := win.Options.Set("monitor-silence", 5); err != nil {
		t.Fatalf("set monitor-silence: %v", err)
	}

	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(epoch),
		State:    state,
	})

	clientConn := pl.dial()
	defer clientConn.Close()
	sendHandshake(t, clientConn)
	time.Sleep(20 * time.Millisecond)

	bellCh := make(chan struct{}, 4)
	go drainClientConn(clientConn, bellCh)

	cm := proto.CommandMsg{Argv: []string{"attach-session", "-t", "monitor-sess"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write attach-session: %v", err)
	}

	select {
	case <-bellCh:
	case <-time.After(3 * time.Second):
		t.Fatal("expected BEL to be sent to client after monitor-silence")
	}

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

// TestMonitorBell verifies that when monitor-bell is on and a pane has a
// pending bell, the window's ActivityFlag is set and a BEL is sent to the client.
func TestMonitorBell(t *testing.T) {
	epoch := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	fp := &testMonitorPane{bellResult: true}
	state, _, win := buildMonitorTestState(t, fp)

	if err := win.Options.Set("monitor-bell", true); err != nil {
		t.Fatalf("set monitor-bell: %v", err)
	}

	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(epoch),
		State:    state,
	})

	clientConn := pl.dial()
	defer clientConn.Close()
	sendHandshake(t, clientConn)
	time.Sleep(20 * time.Millisecond)

	bellCh := make(chan struct{}, 4)
	go drainClientConn(clientConn, bellCh)

	cm := proto.CommandMsg{Argv: []string{"attach-session", "-t", "monitor-sess"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write attach-session: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		if win.ActivityFlag {
			break
		}
	}

	sigs <- fakeSignal("SIGTERM")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within timeout after SIGTERM")
	}

	if !win.ActivityFlag {
		t.Fatal("expected ActivityFlag to be set after monitor-bell sweep")
	}
	select {
	case <-bellCh:
	default:
		t.Fatal("expected BEL to be sent to client after monitor-bell")
	}
}

// TestActivityFlagClearedOnSelect verifies that selecting a window clears its
// ActivityFlag.
func TestActivityFlagClearedOnSelect(t *testing.T) {
	state := session.NewServer()
	sess := session.NewSession(session.SessionID("$1"), "flag-sess", state.Options)
	paneID := session.PaneID(1)
	win := session.NewWindow(session.WindowID("@1"), "main", sess.Options)
	win.AddPane(paneID, &testFakePane{})
	win.Layout = layout.New(80, 24, paneID)
	win.ActivityFlag = true
	sess.AddWindow(win)
	state.AddSession(sess)

	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

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
	time.Sleep(20 * time.Millisecond)

	go func() {
		for {
			if _, _, err := proto.ReadMsg(clientConn); err != nil {
				return
			}
		}
	}()

	// Target "flag-sess:main" resolves by session name and window name.
	cm := proto.CommandMsg{Argv: []string{"select-window", "-t", "flag-sess:main"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write select-window: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	sigs <- fakeSignal("SIGTERM")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within timeout after SIGTERM")
	}

	if win.ActivityFlag {
		t.Fatal("expected ActivityFlag to be cleared after select-window")
	}
}

// ─── fakeOverlay ─────────────────────────────────────────────────────────────

// fakeOverlay is a test-only modes.ClientOverlay that records received keys
// and renders every cell as '#'. If closeKey is set, that key returns
// modes.CloseMode(); all other keys return modes.Consumed().
// keyCh is signaled (non-blocking) each time Key is called.
// closedCh is signaled once when Close is called.
type fakeOverlay struct {
	mu           sync.Mutex
	receivedKeys []keys.Key
	closeKey     keys.KeyCode // if non-zero, that key triggers KindCloseMode
	rect         modes.Rect
	closed       bool
	keyCh        chan struct{} // buffered; signaled on each Key call
	closedCh     chan struct{} // closed once when Close is called
	closeOnce    sync.Once
}

func newFakeOverlay(rect modes.Rect, closeKey keys.KeyCode) *fakeOverlay {
	return &fakeOverlay{
		rect:     rect,
		closeKey: closeKey,
		keyCh:    make(chan struct{}, 8),
		closedCh: make(chan struct{}),
	}
}

func (f *fakeOverlay) Rect() modes.Rect { return f.rect }

func (f *fakeOverlay) Render(dst []modes.Cell) {
	for i := range dst {
		dst[i] = modes.Cell{Char: '#'}
	}
}

func (f *fakeOverlay) Key(k keys.Key) modes.Outcome {
	f.mu.Lock()
	f.receivedKeys = append(f.receivedKeys, k)
	closeMode := f.closeKey != 0 && k.Code == f.closeKey
	f.mu.Unlock()
	select {
	case f.keyCh <- struct{}{}:
	default:
	}
	if closeMode {
		return modes.CloseMode()
	}
	return modes.Consumed()
}

func (f *fakeOverlay) Mouse(_ keys.MouseEvent) modes.Outcome { return modes.Consumed() }
func (f *fakeOverlay) CaptureFocus() bool                    { return true }

func (f *fakeOverlay) Close() {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	f.closeOnce.Do(func() { close(f.closedCh) })
}

func (f *fakeOverlay) ReceivedKeys() []keys.Key {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]keys.Key, len(f.receivedKeys))
	copy(out, f.receivedKeys)
	return out
}

func (f *fakeOverlay) IsClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// ─── Overlay tests ────────────────────────────────────────────────────────────

// TestOverlayReceivesKey verifies that key events are routed to the active
// overlay when it captures focus, instead of falling through to the key table.
func TestOverlayReceivesKey(t *testing.T) {
	state := session.NewServer()
	sess := session.NewSession(session.SessionID("$1"), "ov-sess", state.Options)
	paneID := session.PaneID(1)
	activePaneCapture := &testCapturingPane{}
	win := session.NewWindow(session.WindowID("@1"), "main", sess.Options)
	win.AddPane(paneID, activePaneCapture)
	win.Layout = layout.New(80, 24, paneID)
	sess.AddWindow(win)
	state.AddSession(sess)

	overlay := newFakeOverlay(modes.Rect{X: 0, Y: 0, Width: 10, Height: 5}, 0)

	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)
	dirtyIDs := make(chan session.ClientID, 16)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
		State:    state,
		OverlayPusher: func(_ session.ClientID) modes.ClientOverlay {
			return overlay
		},
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
	time.Sleep(20 * time.Millisecond)

	go func() {
		for {
			if _, _, err := proto.ReadMsg(clientConn); err != nil {
				return
			}
		}
	}()

	cm := proto.CommandMsg{Argv: []string{"attach-session", "-t", "ov-sess"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write attach-session: %v", err)
	}
	select {
	case <-dirtyIDs:
	case <-time.After(2 * time.Second):
		t.Fatal("OnDirty not called after attach-session")
	}

	// Send keystroke 'a' as stdin.
	stdinMsg := proto.StdinMsg{Data: []byte("a")}
	if err := proto.WriteMsg(clientConn, proto.MsgStdin, stdinMsg.Encode()); err != nil {
		t.Fatalf("write MsgStdin: %v", err)
	}

	// Wait for the overlay's Key method to be called rather than relying on
	// OnDirty ordering, which can interleave with prior events.
	select {
	case <-overlay.keyCh:
	case <-time.After(2 * time.Second):
		t.Fatal("overlay did not receive key within timeout")
	}

	// The overlay should have received 'a'; the pane should not.
	received := overlay.ReceivedKeys()
	if len(received) == 0 {
		t.Fatal("expected overlay to receive at least one key, got none")
	}
	if received[0].Code != keys.KeyCode('a') {
		t.Errorf("expected overlay to receive key 'a' (code %d), got code %d", keys.KeyCode('a'), received[0].Code)
	}
	if written := activePaneCapture.Written(); len(written) != 0 {
		t.Errorf("expected active pane to receive no input (overlay captures focus), got %q", written)
	}

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

// TestOverlayCloseMode verifies that when an overlay returns KindCloseMode
// for a key, the overlay is removed from the stack and Close is called on it.
func TestOverlayCloseMode(t *testing.T) {
	state := session.NewServer()
	sess := session.NewSession(session.SessionID("$1"), "close-sess", state.Options)
	paneID := session.PaneID(1)
	win := session.NewWindow(session.WindowID("@1"), "main", sess.Options)
	win.AddPane(paneID, &testFakePane{})
	win.Layout = layout.New(80, 24, paneID)
	sess.AddWindow(win)
	state.AddSession(sess)

	// Overlay returns KindCloseMode when 'q' is pressed.
	overlay := newFakeOverlay(modes.Rect{X: 0, Y: 0, Width: 10, Height: 5}, keys.KeyCode('q'))

	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)
	dirtyIDs := make(chan session.ClientID, 16)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
		State:    state,
		OverlayPusher: func(_ session.ClientID) modes.ClientOverlay {
			return overlay
		},
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
	time.Sleep(20 * time.Millisecond)

	go func() {
		for {
			if _, _, err := proto.ReadMsg(clientConn); err != nil {
				return
			}
		}
	}()

	cm := proto.CommandMsg{Argv: []string{"attach-session", "-t", "close-sess"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write attach-session: %v", err)
	}
	select {
	case <-dirtyIDs:
	case <-time.After(2 * time.Second):
		t.Fatal("OnDirty not called after attach-session")
	}

	// Send 'q' to trigger CloseMode.
	stdinMsg := proto.StdinMsg{Data: []byte("q")}
	if err := proto.WriteMsg(clientConn, proto.MsgStdin, stdinMsg.Encode()); err != nil {
		t.Fatalf("write MsgStdin: %v", err)
	}

	// Wait for the overlay's Key method to be called (which triggers Close).
	select {
	case <-overlay.keyCh:
	case <-time.After(2 * time.Second):
		t.Fatal("overlay did not receive key within timeout")
	}

	// Wait for Close to be called (closedCh is closed by Close()).
	select {
	case <-overlay.closedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("expected overlay.Close() to have been called after KindCloseMode")
	}

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

// TestOverlayRendered verifies that the overlay's Render output is included
// in the composed frame sent to the client.
func TestOverlayRendered(t *testing.T) {
	state := session.NewServer()
	sess := session.NewSession(session.SessionID("$1"), "render-sess", state.Options)
	paneID := session.PaneID(1)
	win := session.NewWindow(session.WindowID("@1"), "main", sess.Options)
	win.AddPane(paneID, &testFakePane{})
	win.Layout = layout.New(80, 24, paneID)
	sess.AddWindow(win)
	state.AddSession(sess)

	// The overlay renders '#' characters over a 10×5 region at the top-left.
	overlay := newFakeOverlay(modes.Rect{X: 0, Y: 0, Width: 10, Height: 5}, 0)

	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
		State:    state,
		OverlayPusher: func(_ session.ClientID) modes.ClientOverlay {
			return overlay
		},
	})

	clientConn := pl.dial()
	defer clientConn.Close()
	sendHandshake(t, clientConn)
	time.Sleep(10 * time.Millisecond)

	// Attach to trigger a render.
	cm := proto.CommandMsg{Argv: []string{"attach-session", "-t", "render-sess"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write attach-session: %v", err)
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

	// The composed frame should contain '#' from the overlay render.
	if !bytes.ContainsRune(sm.Data, '#') {
		preview := sm.Data
		if len(preview) > 200 {
			preview = preview[:200]
		}
		t.Fatalf("expected '#' from overlay in ANSI output, got: %q", preview)
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

// sendHandshakeAs writes a VERSION + IDENTIFY sequence to conn, reporting the
// connecting client as having the given Unix username via the USER env variable.
func sendHandshakeAs(t *testing.T, conn net.Conn, username string) {
	t.Helper()

	if err := proto.WriteMsg(conn, proto.MsgVersion, (proto.VersionMsg{Version: 1}).Encode()); err != nil {
		t.Fatalf("sendHandshakeAs: write VERSION: %v", err)
	}

	msgs := []struct {
		t proto.MsgType
		p []byte
	}{
		{proto.MsgIdentifyFlags, (proto.IdentifyFlagsMsg{Flags: 0}).Encode()},
		{proto.MsgIdentifyTerm, (proto.IdentifyTermMsg{Term: "xterm-256color"}).Encode()},
		{proto.MsgIdentifyTTYName, (proto.IdentifyTTYNameMsg{TTYName: "/dev/pts/0"}).Encode()},
		{proto.MsgIdentifyCWD, (proto.IdentifyCWDMsg{CWD: "/tmp"}).Encode()},
		{proto.MsgIdentifyEnviron, (proto.IdentifyEnvironMsg{Pairs: []string{
			"TERM=xterm-256color",
			"HOME=/home/" + username,
			"USER=" + username,
		}}).Encode()},
		{proto.MsgIdentifyClientPID, (proto.IdentifyClientPIDMsg{PID: 9999}).Encode()},
		{proto.MsgIdentifyFeatures, (proto.IdentifyFeaturesMsg{Features: 0}).Encode()},
		{proto.MsgIdentifyDone, (proto.IdentifyDoneMsg{}).Encode()},
	}
	for _, m := range msgs {
		if err := proto.WriteMsg(conn, m.t, m.p); err != nil {
			t.Fatalf("sendHandshakeAs: write %s: %v", m.t, err)
		}
	}
}

// readNextMsg reads one message from conn with a 2-second deadline.
func readNextMsg(t *testing.T, conn net.Conn) (proto.MsgType, []byte) {
	t.Helper()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("readNextMsg: set deadline: %v", err)
	}
	defer conn.SetDeadline(time.Time{}) //nolint:errcheck
	msgType, payload, err := proto.ReadMsg(conn)
	if err != nil {
		t.Fatalf("readNextMsg: %v", err)
	}
	return msgType, payload
}

// TestACLDenyAll verifies that DenyAllClients causes subsequent connection
// attempts to receive an EXIT message and be rejected.
func TestACLDenyAll(t *testing.T) {
	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	st := session.NewServer()
	st.ACLDenyAll = true

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
		State:    st,
	})

	clientConn := pl.dial()
	defer clientConn.Close()

	sendHandshakeAs(t, clientConn, "alice")

	// The server should respond with EXIT(1).
	msgType, payload := readNextMsg(t, clientConn)
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
		t.Fatal("Run() did not return within timeout after SIGTERM")
	}
}

// TestACLDenyUser verifies that SetServerAccess(user, false, false) rejects
// connections from that user with an EXIT message.
func TestACLDenyUser(t *testing.T) {
	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	st := session.NewServer()
	st.ACL["bob"] = false

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
		State:    st,
	})

	// bob should be rejected.
	bobConn := pl.dial()
	defer bobConn.Close()
	sendHandshakeAs(t, bobConn, "bob")

	msgType, payload := readNextMsg(t, bobConn)
	if msgType != proto.MsgExit {
		t.Fatalf("expected MsgExit for bob, got %s", msgType)
	}
	var em proto.ExitMsg
	if err := em.Decode(payload); err != nil {
		t.Fatalf("decode EXIT: %v", err)
	}
	if em.Code != 1 {
		t.Fatalf("expected exit code 1 for bob, got %d", em.Code)
	}

	// alice (not in ACL) should be accepted normally.
	aliceConn := pl.dial()
	defer aliceConn.Close()
	sendHandshakeAs(t, aliceConn, "alice")

	// Give the server time to process the identify sequence and set up the session.
	time.Sleep(20 * time.Millisecond)

	if err := aliceConn.SetDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	cm := proto.CommandMsg{Argv: []string{"list-sessions"}}
	if err := proto.WriteMsg(aliceConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write list-sessions: %v", err)
	}
	// Confirm we can read a response without io.EOF (which would indicate the
	// server closed the connection).
	mt, _, err := proto.ReadMsg(aliceConn)
	if err != nil {
		t.Fatalf("alice: read response: %v", err)
	}
	if mt == proto.MsgExit {
		t.Fatal("alice: unexpectedly received EXIT after list-sessions")
	}
	aliceConn.SetDeadline(time.Time{}) //nolint:errcheck

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

// TestACLReadOnly verifies that SetServerAccess(user, true, false) accepts a
// connection but mutating commands return a permission-denied error.
func TestACLReadOnly(t *testing.T) {
	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	st := session.NewServer()
	// carol has read access but no write access.
	st.ACL["carol"] = true
	// ACLWriteAccess intentionally not set for carol.

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
		State:    st,
	})

	carolConn := pl.dial()
	defer carolConn.Close()
	sendHandshakeAs(t, carolConn, "carol")

	// Give the server time to process the identify sequence and set up the session.
	time.Sleep(20 * time.Millisecond)

	if err := carolConn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	// A mutating command (new-session) should be rejected with permission denied.
	cm := proto.CommandMsg{Argv: []string{"new-session", "-s", "ro-test"}}
	if err := proto.WriteMsg(carolConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write new-session: %v", err)
	}

	// Read messages until we find a MsgStdout containing the error.
	found := false
	for i := 0; i < 10; i++ {
		mt, p, err := proto.ReadMsg(carolConn)
		if err != nil {
			t.Fatalf("read response: %v", err)
		}
		if mt == proto.MsgExit {
			t.Fatal("read-only client received EXIT instead of error message")
		}
		if mt == proto.MsgStdout {
			var sm proto.StdoutMsg
			if err := sm.Decode(p); err != nil {
				t.Fatalf("decode StdoutMsg: %v", err)
			}
			if strings.Contains(string(sm.Data), "permission denied") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("expected 'permission denied' error for read-only client, not received")
	}
	carolConn.SetDeadline(time.Time{}) //nolint:errcheck

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
