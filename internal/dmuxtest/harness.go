package dmuxtest

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/script"
	"github.com/dhamidi/dmux/internal/server"
)

// goroutineLeakSlack is the grace window for short-lived runtime
// goroutines that come and go (GC worker starts, netpoll restarts,
// etc.). Crossing this number over the pre-spawn baseline is what
// the harness calls a leak.
const goroutineLeakSlack = 5

// dialRetryInterval is the polling gap between Dial attempts when
// waiting for the freshly-spawned server's socket to accept
// connections. Matches the 50ms/40-tries convention used in
// internal/socket.DialOrStart — a 2s total window is plenty of
// head-room for an in-process server on a loaded CI host.
const dialRetryInterval = 50 * time.Millisecond

// dialRetryAttempts bounds the wait for the server to come up.
const dialRetryAttempts = 40

// serverShutdownTimeout bounds how long Close will wait for the
// server goroutine to return after kill-server lands. Three seconds
// is generous: an in-process server has nothing to clean up but the
// vt runtime and a per-pane goroutine or two.
const serverShutdownTimeout = 3 * time.Second

// Harness drives a real dmux server in the test process. Each
// harness owns its own tempdir + socket, so harnesses are safely
// parallel: no port to collide on, no shared global state. Created
// via SpawnServer, destroyed via t.Cleanup — callers never call
// Close directly.
type Harness struct {
	t          *testing.T
	socketPath string
	dir        string
	baseline   int // runtime.NumGoroutine before the server started

	serverDone chan error // closed by the server-runner goroutine on return
	closed     bool
}

// SpawnServer starts a dmux server in the current process backed by
// a tempdir socket. The returned Harness is pinned to t via
// t.Cleanup; callers drive it through Run and let the cleanup tear
// it down on test exit.
//
// On any setup failure (socket never came up, server.Run returned
// before we dialed) the test fails with t.Fatalf — no half-spawned
// harness is returned.
func SpawnServer(t *testing.T) *Harness {
	t.Helper()

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "dmux.sock")

	h := &Harness{
		t:          t,
		socketPath: socketPath,
		dir:        dir,
		baseline:   runtime.NumGoroutine(),
		serverDone: make(chan error, 1),
	}

	go func() {
		h.serverDone <- server.Run(socketPath)
	}()

	if err := waitForSocket(socketPath, h.serverDone); err != nil {
		t.Fatalf("dmuxtest: server did not come up: %v", err)
	}

	t.Cleanup(h.Close)
	return h
}

// waitForSocket polls net.Dial until the server is accepting, the
// server goroutine has returned an error, or the retry budget
// expires. A dial success is immediately closed — the point is
// readiness, not reuse.
func waitForSocket(path string, serverDone <-chan error) error {
	for i := 0; i < dialRetryAttempts; i++ {
		select {
		case err := <-serverDone:
			if err != nil {
				return fmt.Errorf("server.Run returned early: %w", err)
			}
			return errors.New("server.Run returned before socket was reachable")
		default:
		}
		conn, err := net.Dial("unix", path)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(dialRetryInterval)
	}
	return fmt.Errorf("socket %s never accepted within %v", path, dialRetryInterval*time.Duration(dialRetryAttempts))
}

// SocketPath returns the AF_UNIX path the harness's server is
// bound to. Useful for tests that want to dial directly instead of
// going through Run.
func (h *Harness) SocketPath() string { return h.socketPath }

// Dialer returns a script.Dialer that opens connections to the
// harness's server. Used by Play, PlayInline, and any test wiring
// the script package directly.
func (h *Harness) Dialer() script.Dialer {
	return func(ctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", h.socketPath)
	}
}

// Run sends one command to the harness's server, parsed as a single
// script line. Returns nil on a CommandResult{Ok}; a *script.CommandError
// wrapping script.ErrCommandFailed on any other status; or a wrapped
// transport error on dial / framing failure.
//
// Run is a thin wrapper around script.Tokenize + script.RunLine so
// the harness's per-line execution stays in lock-step with the
// production script runner.
func (h *Harness) Run(cmdLine string) error {
	argv, err := script.Tokenize(cmdLine)
	if err != nil {
		return fmt.Errorf("dmuxtest: tokenize %q: %w", cmdLine, err)
	}
	if len(argv) == 0 {
		return fmt.Errorf("dmuxtest: empty command line")
	}
	return script.RunLine(context.Background(), h.Dialer(), argv)
}

// Close tears down the harness. Safe to call multiple times (the
// t.Cleanup registration means SpawnServer's caller usually does
// NOT call it directly); the second and subsequent calls are
// no-ops.
//
// Tear-down sequence:
//
//  1. Send kill-server. Any transport / command error is swallowed:
//     the server may already be gone (another Close, or the test
//     called kill-server manually), and Close's job is to make the
//     state deterministic, not to report every path by which the
//     state got there.
//  2. Wait up to serverShutdownTimeout for the server goroutine
//     to return. A timeout fails the test — a stuck server means
//     real state was leaked.
//  3. Goroutine-leak check: if NumGoroutine is above the pre-spawn
//     baseline by more than goroutineLeakSlack, dump every
//     goroutine's stack and fail the test.
func (h *Harness) Close() {
	if h.closed {
		return
	}
	h.closed = true

	// Best-effort kill. Ignore the error — the only reason we care
	// is to make the server return from Run, and if it is already
	// gone that job is already done.
	_ = h.Run("kill-server")

	select {
	case err := <-h.serverDone:
		if err != nil {
			h.t.Errorf("dmuxtest: server.Run returned error: %v", err)
		}
	case <-time.After(serverShutdownTimeout):
		h.t.Errorf("dmuxtest: server.Run did not return within %v", serverShutdownTimeout)
		return
	}

	// Small settle window for per-connection teardown goroutines
	// to observe ctx cancellation and return. Without this, the
	// leak check races the teardown and flags false positives.
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= h.baseline+goroutineLeakSlack {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	now := runtime.NumGoroutine()
	if now > h.baseline+goroutineLeakSlack {
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		h.t.Errorf("dmuxtest: goroutine leak: baseline=%d now=%d slack=%d\n%s",
			h.baseline, now, goroutineLeakSlack, buf[:n])
	}
}
