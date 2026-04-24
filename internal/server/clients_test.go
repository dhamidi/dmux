//go:build unix

package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/options"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/session"
	"github.com/dhamidi/dmux/internal/vt"

	// Blank imports register the commands the synthetic client's
	// Spawn drives via its CommandList (attach-session) and that
	// tests dispatch directly.
	_ "github.com/dhamidi/dmux/internal/cmd/attachsession"
)

// newTestState constructs a serverState wired end-to-end: a real
// vt.Runtime, an empty registry, a fresh clientManager. Cleanup
// tears everything down on test exit. No socket is bound — callers
// drive clientManager directly instead of Accept + handle.
func newTestState(t *testing.T) *serverState {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	rt, err := vt.NewRuntime(ctx)
	if err != nil {
		t.Fatalf("vt.NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })

	state := &serverState{
		ctx:            ctx,
		cancel:         cancel,
		rt:             rt,
		registry:       session.NewRegistry(),
		serverOptions:  options.NewServerOptions(),
		serverSessions: make(map[session.ID]*serverSession),
		attached:       make(map[uint64]*attachedClient),
	}
	state.clients = newClientManager(state)
	state.installDefaultKeyTables()

	t.Cleanup(state.shutdownRegistry)
	return state
}

// newTestSession wraps createSession for the tests: /bin/cat is a
// predictable long-running child that echoes its input on the pty.
// Cleanup is handled by state.shutdownRegistry.
func newTestSession(t *testing.T, state *serverState) *session.Session {
	t.Helper()
	state.registryMu.Lock()
	defer state.registryMu.Unlock()
	sess, err := state.createSession("test", "/", nil, "xterm", 80, 24)
	if err != nil {
		t.Fatalf("createSession: %v", err)
	}
	return sess
}

func TestSpawnKillRoundTrip(t *testing.T) {
	state := newTestState(t)
	newTestSession(t, state)

	ref, err := state.clients.Spawn("", 0, 0)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if ref == "" {
		t.Fatalf("Spawn returned empty ref")
	}

	// Kill must drain the synthetic client's goroutines. A 2s wait
	// inside Kill plus a test-side deadline catches any regression
	// that would leak the client.Run goroutine.
	done := make(chan error, 1)
	go func() { done <- state.clients.Kill(ref) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Kill: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Kill did not return within 3s")
	}
}

func TestKillStaleRefWrapsSentinel(t *testing.T) {
	state := newTestState(t)
	err := state.clients.Kill("ghost")
	if err == nil {
		t.Fatalf("Kill of unknown ref returned nil")
	}
	if !errors.Is(err, cmd.ErrStaleClient) {
		t.Fatalf("Kill error %v does not wrap ErrStaleClient", err)
	}
}

func TestInjectStaleRefWrapsSentinel(t *testing.T) {
	state := newTestState(t)
	err := state.clients.Inject("ghost", []byte("x"))
	if err == nil {
		t.Fatalf("Inject on unknown ref returned nil")
	}
	if !errors.Is(err, cmd.ErrStaleClient) {
		t.Fatalf("Inject error %v does not wrap ErrStaleClient", err)
	}
}

func TestInjectReachesPane(t *testing.T) {
	state := newTestState(t)
	sess := newTestSession(t, state)
	p := sess.CurrentWindow().ActivePane()

	ref, err := state.clients.Spawn("", 0, 0)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() { _ = state.clients.Kill(ref) })

	// Wait for the synthetic client to finish its handshake +
	// attach-session drain so Inject lands in pump (where Input
	// frames are forwarded to the pane). Polling the session's
	// attached-client map is the cheapest observation: register()
	// runs once pump entered.
	waitAttached(t, state, 3*time.Second)

	// Feed a unique marker string the shell will echo back on the pty.
	// The pty translates \n to \r\n on output (default termios), so
	// the needle is just the marker sans newline — we don't care
	// whether the prompt or the bash echo lands first, only that the
	// marker appears in the byte stream.
	marker := []byte("DMUX_TEST_MARKER")
	if err := state.clients.Inject(ref, append(marker, '\n')); err != nil {
		t.Fatalf("Inject: %v", err)
	}

	// The shell echoes stdin back on the pty by default (ECHO termios
	// flag). waitForPaneBytes drains pane.Bytes until the marker
	// appears or the deadline expires. Reading the pane directly
	// skips the renderer, which would wrap the bytes in VT escape
	// sequences.
	if !waitForPaneBytes(t, p, marker, 3*time.Second) {
		t.Fatalf("pane did not observe injected marker")
	}
}

// waitAttached polls state.attached until it sees a non-empty map
// or the deadline expires. The clientManager's Spawn is fire-and-
// forget (handshake is not awaited), so tests that need register()
// to have run must poll for it. Not part of the public surface.
func waitAttached(t *testing.T, state *serverState, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		state.attachedMu.Lock()
		n := len(state.attached)
		state.attachedMu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no client attached within deadline")
}

// waitForPaneBytes drains the pane's bytesCh until needle appears or
// the deadline expires. /bin/cat echoes stdin, so bytes written via
// Inject reappear on the same channel.
func waitForPaneBytes(t *testing.T, p *pane.Pane, needle []byte, d time.Duration) bool {
	t.Helper()
	deadline := time.After(d)
	var seen []byte
	for {
		select {
		case chunk, ok := <-p.Bytes():
			if !ok {
				return false
			}
			seen = append(seen, chunk...)
			if containsBytes(seen, needle) {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// TestClientSelfExitReaps proves the driver goroutine reaps its own
// ref when client.Run returns on its own. The test spawns against an
// empty registry: attach-session fails, handle() returns, the
// server-side pipe closes, and client.Run wakes up with EOF. After
// that, Kill and Inject on the same ref must see the stale-ref path.
// Without the reap, the entry would linger and Kill would return nil.
func TestClientSelfExitReaps(t *testing.T) {
	state := newTestState(t)
	// Intentionally no session: attach-session has nothing to attach
	// to and the server tears the connection down.

	ref, err := state.clients.Spawn("", 0, 0)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Poll the manager's map directly for the ref being reaped. Using
	// Kill to observe would race reap against Kill's own teardown
	// (whichever wins the lock removes the entry), so we'd be testing
	// "something removed the entry" rather than "the driver reaped it."
	deadline := time.Now().Add(3 * time.Second)
	for {
		state.clients.mu.Lock()
		_, present := state.clients.clients[ref]
		state.clients.mu.Unlock()
		if !present {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("driver goroutine did not reap ref within deadline")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := state.clients.Kill(ref); !errors.Is(err, cmd.ErrStaleClient) {
		t.Fatalf("Kill after self-exit: got %v, want wrapped ErrStaleClient", err)
	}
	if err := state.clients.Inject(ref, []byte("x")); !errors.Is(err, cmd.ErrStaleClient) {
		t.Fatalf("Inject after self-exit: got %v, want wrapped ErrStaleClient", err)
	}
}

func TestParseProfile(t *testing.T) {
	cases := []struct {
		in      string
		want    uint8
		wantErr bool
	}{
		{"", 0, false},
		{"0", 0, false},
		{"42", 42, false},
		{"255", 255, false},
		{"256", 0, true},
		{"abc", 0, true},
	}
	for _, c := range cases {
		got, err := parseProfile(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("parseProfile(%q) err=%v, wantErr=%v", c.in, err, c.wantErr)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("parseProfile(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
