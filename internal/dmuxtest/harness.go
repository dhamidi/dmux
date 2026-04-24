package dmuxtest

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/server"
	"github.com/dhamidi/dmux/internal/xio"
)

// ErrCommandFailed is the sentinel returned by (*Harness).Run when a
// scenario command completed but the server answered with a
// non-ok CommandResult. Callers use errors.Is to distinguish this
// from transport failures (connection drops, socket gone) — the
// latter are wrapped directly without going through this sentinel.
var ErrCommandFailed = errors.New("dmuxtest: command failed")

// CommandError carries the structured detail for a non-ok
// CommandResult. Err always wraps ErrCommandFailed so callers can
// errors.Is, and the ID / Status / Message fields let diagnostics
// pick out the specifics.
type CommandError struct {
	ID      uint32
	Status  proto.CommandStatus
	Message string
}

// Error reports the command result in a form that chains cleanly
// with xio / proto errors above it.
func (e *CommandError) Error() string {
	return fmt.Sprintf("dmuxtest: command %d: %s: %s", e.ID, e.Status, e.Message)
}

// Unwrap returns ErrCommandFailed so errors.Is works.
func (e *CommandError) Unwrap() error { return ErrCommandFailed }

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

// Run sends one command to the harness's server. cmdLine is parsed
// by the package's tokenizer (whitespace + double-quoted runs with
// Go-style escapes); the resulting argv becomes a single-element
// CommandList with ID 1. Run returns on the first of:
//
//   - CommandResult arrives with Status = StatusOk (nil return).
//   - CommandResult arrives with Status != StatusOk (CommandError
//     wrapping ErrCommandFailed).
//   - Connection closes before a result arrives (transport error).
//
// Run deliberately does not read more than one frame past the
// result: kill-server races the Exit frame against connection
// teardown, so "result then socket closes" is a normal outcome for
// that command. The call sites that chain multiple commands do so
// by calling Run multiple times.
func (h *Harness) Run(cmdLine string) error {
	argv, err := tokenize(cmdLine)
	if err != nil {
		return fmt.Errorf("dmuxtest: tokenize %q: %w", cmdLine, err)
	}
	if len(argv) == 0 {
		return fmt.Errorf("dmuxtest: empty command line")
	}

	conn, err := net.Dial("unix", h.socketPath)
	if err != nil {
		return fmt.Errorf("dmuxtest: dial %s: %w", h.socketPath, err)
	}
	defer conn.Close()

	fr := xio.NewReader(conn)
	fw := xio.NewWriter(conn)

	cwd, _ := os.Getwd()
	ident := &proto.Identify{
		ProtocolVersion: proto.ProtocolVersion,
		Profile:         0,
		InitialCols:     0,
		InitialRows:     0,
		Cwd:             cwd,
		TTYName:         "",
		TermEnv:         "",
		Env:             nil,
	}
	if err := fw.WriteFrame(ident); err != nil {
		return fmt.Errorf("dmuxtest: write Identify: %w", err)
	}
	if err := fw.WriteFrame(&proto.CommandList{
		Commands: []proto.Command{{ID: 1, Argv: argv}},
	}); err != nil {
		return fmt.Errorf("dmuxtest: write CommandList: %w", err)
	}

	// Read frames until we see the single CommandResult for our
	// command. After that, any follow-up behaviour is
	// command-specific:
	//
	//   - kill-server and detach-family commands follow up with an
	//     Exit frame (or close the socket). We do not wait for it
	//     — the scenario has already gotten its ack, and the next
	//     Run will dial a fresh connection.
	//   - new-session / attach-session transition into the server's
	//     pump loop and start emitting Output frames forever. We
	//     also do not wait: closing our side of the socket makes
	//     the server's pump observe EOF and clean up the attach
	//     without our having to render anything.
	//
	// The single CommandResult is thus both the ack AND the signal
	// to return; everything past it is server bookkeeping that our
	// defer conn.Close will tell the server to abandon.
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return fmt.Errorf("dmuxtest: connection closed before CommandResult")
			}
			return fmt.Errorf("dmuxtest: read frame: %w", err)
		}
		switch m := f.(type) {
		case *proto.CommandResult:
			if m.Status != proto.StatusOk {
				return &CommandError{
					ID:      m.ID,
					Status:  m.Status,
					Message: m.Message,
				}
			}
			return nil
		default:
			// Other frame types (Output, Beep, CapsUpdate, Exit) can
			// precede or follow the result on attach paths; skip
			// them. A protocol-error Exit that arrives before any
			// CommandResult is handled by the read-error arm above
			// once the server closes the socket.
		}
	}
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
