package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/dhamidi/dmux/internal/client"
	"github.com/dhamidi/dmux/internal/cmd"
	// Synthetic clients send a hard-coded attach-session
	// CommandList from Spawn. Any binary that pulls in this package
	// (production dmux, dmuxtest, future agent harnesses) needs the
	// command registered or handle() rejects the CommandList as a
	// protocol error and tears the synthetic conn down before the
	// caller can drive it. Blank-importing here moves the dependency
	// to the only file that actually relies on the command name.
	_ "github.com/dhamidi/dmux/internal/cmd/attachsession"
	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/tty"
)

// clientManager is the real, in-process cmd.ClientManager. It wires a
// synthetic client into the same server it lives in through net.Pipe()
// so every byte travels the real wire protocol (Identify +
// CommandList + Input/Output pump). Scenario tests and future AI-agent
// hooks drive these clients from Go code instead of a real tty.
//
// The manager is the long-lived handle: one per serverState, created
// in Run, torn down in shutdownRegistry. Each spawned client lives in
// the clients map keyed by a monotonic decimal ref; Kill removes the
// entry and cancels the client's context, waiting for client.Run to
// return before reporting success. Inject pushes bytes into the
// synthetic terminal's Read queue; they surface server-side as
// proto.Input frames on the pipe.
type clientManager struct {
	state *serverState

	mu      sync.Mutex
	clients map[string]*syntheticClient
	nextRef uint64
}

// newClientManager constructs the manager owned by state. Must be
// called before state's Accept loop hands connections out so the
// clients field is non-nil on first lookup.
func newClientManager(state *serverState) *clientManager {
	return &clientManager{
		state:   state,
		clients: make(map[string]*syntheticClient),
	}
}

// syntheticClient owns one side of a net.Pipe() pair plus the
// goroutines that drive both ends: the server half running handle(),
// and the client half running client.Run(). The syntheticTerminal is
// the scripted I/O surface client.Run reads from and writes to. done
// closes when client.Run returns so Kill can wait for full teardown.
type syntheticClient struct {
	serverConn net.Conn
	clientConn net.Conn
	term       *syntheticTerminal
	cancel     context.CancelFunc
	done       chan struct{}
}

// killWait is the upper bound Kill waits for client.Run to return.
// If wedged for longer, Kill returns nil anyway so one stuck
// goroutine does not block the server goroutine indefinitely — the
// orphan will leak but the caller can keep making progress.
const killWait = 2 * time.Second

// Spawn creates a synthetic client attached to the server's
// most-recent session (matching `dmux attach` with no target). The
// profile string is parsed as a uint8 termcaps profile number;
// empty string means 0 (Unknown). Zero cols or rows fall back to
// 80x24. The returned ref is a decimal string; handshake progress is
// NOT awaited — callers that need to synchronize on Identify +
// CommandResult use the `wait` command.
func (m *clientManager) Spawn(profile string, cols, rows int) (string, error) {
	prof, err := parseProfile(profile)
	if err != nil {
		return "", err
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	serverEnd, clientEnd := net.Pipe()
	term := newSyntheticTerminal(cols, rows)

	runCtx, cancel := context.WithCancel(m.state.ctx)
	sc := &syntheticClient{
		serverConn: serverEnd,
		clientConn: clientEnd,
		term:       term,
		cancel:     cancel,
		done:       make(chan struct{}),
	}

	m.mu.Lock()
	m.nextRef++
	ref := strconv.FormatUint(m.nextRef, 10)
	m.clients[ref] = sc
	m.mu.Unlock()

	// Server-side handler: drive the server end exactly like any
	// other accepted connection. A failure here surfaces to the
	// client side as a pipe read error; handle() already emits Exit
	// frames on its own protocol failures.
	go func() {
		defer serverEnd.Close()
		_ = handle(serverEnd, m.state)
	}()

	// Client-side driver. attach-session picks the most-recent
	// session so the scenario-test contract matches bare `dmux
	// attach`. A missing session comes back as a CommandResult
	// error which client.Run logs and continues from — Spawn does
	// not block on it. When client.Run returns on its own (the
	// server hung up, attach-session failed against an empty
	// registry, etc.) reap removes the ref from the map and closes
	// the terminal so a later Kill surfaces ErrStaleClient and a
	// later Inject cannot push into a dead buffer.
	go func() {
		defer close(sc.done)
		defer clientEnd.Close()
		opts := client.Options{
			Profile: prof,
			Commands: []proto.Command{
				{ID: 1, Argv: []string{"attach-session"}},
			},
		}
		_, _ = client.Run(runCtx, clientEnd, term, opts)
		m.reap(ref)
	}()

	return ref, nil
}

// Kill tears down the synthetic client named by ref. A ref that no
// longer resolves returns an error wrapping cmd.ErrStaleClient so
// callers that keep their own bookkeeping (see
// internal/cmd/client.execKill) can treat "already gone" as success.
// Kill removes the entry under the lock before doing any teardown
// work so a racing Kill on the same ref sees the stale-ref path.
func (m *clientManager) Kill(ref string) error {
	m.mu.Lock()
	sc, ok := m.clients[ref]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("server: kill client %s: %w", ref, cmd.ErrStaleClient)
	}
	delete(m.clients, ref)
	m.mu.Unlock()

	sc.term.close()
	sc.cancel()
	_ = sc.clientConn.Close()
	_ = sc.serverConn.Close()

	select {
	case <-sc.done:
	case <-time.After(killWait):
	}
	return nil
}

// Inject pushes bytes into the synthetic terminal's Read queue so
// client.Run's stdin goroutine forwards them as proto.Input frames
// on the pipe. A ref that no longer resolves, or one whose terminal
// has already been closed (the client exited), returns an error
// wrapping cmd.ErrStaleClient.
func (m *clientManager) Inject(ref string, b []byte) error {
	m.mu.Lock()
	sc, ok := m.clients[ref]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("server: inject into client %s: %w", ref, cmd.ErrStaleClient)
	}
	if err := sc.term.push(b); err != nil {
		return fmt.Errorf("server: inject into client %s: %w", ref, err)
	}
	return nil
}

// reap removes ref from the clients map and closes the terminal.
// Idempotent: a no-op if ref has already been removed (e.g. by a
// racing Kill). Called by Spawn's driver goroutine after client.Run
// returns on its own, so a subsequent Kill/Inject on the same ref
// hits the stale-ref path instead of operating on a half-dead entry.
func (m *clientManager) reap(ref string) {
	m.mu.Lock()
	sc, ok := m.clients[ref]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.clients, ref)
	m.mu.Unlock()
	sc.term.close()
}

// shutdown tears every remaining client down. Called from
// serverState.shutdownRegistry on server exit so no goroutine leaks
// past the Run return. After shutdown the manager's map is empty and
// the clients' done channels have all closed (or the 2s wait
// elapsed).
func (m *clientManager) shutdown() {
	m.mu.Lock()
	refs := make([]string, 0, len(m.clients))
	for ref := range m.clients {
		refs = append(refs, ref)
	}
	m.mu.Unlock()

	for _, ref := range refs {
		_ = m.Kill(ref)
	}
}

// parseProfile decodes the spawn command's -F flag value into a
// termcaps profile number. Empty string is Unknown (0). An
// out-of-range or non-numeric value returns an error — the caller
// surfaces it on the CommandResult.
func parseProfile(s string) (uint8, error) {
	if s == "" {
		return 0, nil
	}
	v, err := strconv.ParseUint(s, 10, 8)
	if err != nil {
		return 0, fmt.Errorf("server: parse profile %q: %w", s, err)
	}
	return uint8(v), nil
}

// syntheticTerminal is client.Terminal driven from Go. Reads block
// on the inCh channel fed by Inject; Writes append to screen under
// a mutex so a future assert command can inspect what the server
// painted. Size is fixed at construction; Resize is a no-op (M1
// does not support synthetic resize yet).
type syntheticTerminal struct {
	cols, rows int

	mu     sync.Mutex
	screen []byte

	inMu     sync.Mutex
	pending  [][]byte
	ready    chan struct{}
	closed   bool
	closedCh chan struct{}

	// resizeCh is a never-firing channel so the resize goroutine in
	// client.Run blocks forever on it. Allocated once per terminal
	// and closed by close() so the goroutine returns on teardown.
	resizeCh chan tty.ResizeEvent
}

func newSyntheticTerminal(cols, rows int) *syntheticTerminal {
	return &syntheticTerminal{
		cols:     cols,
		rows:     rows,
		ready:    make(chan struct{}, 1),
		closedCh: make(chan struct{}),
		resizeCh: make(chan tty.ResizeEvent),
	}
}

// Read blocks until push delivers bytes or close returns io.EOF.
// Multiple pending chunks are drained one at a time; a short read is
// fine (client.Run copies per-chunk and re-enters Read).
func (t *syntheticTerminal) Read(p []byte) (int, error) {
	for {
		t.inMu.Lock()
		if len(t.pending) > 0 {
			chunk := t.pending[0]
			n := copy(p, chunk)
			if n < len(chunk) {
				t.pending[0] = chunk[n:]
			} else {
				t.pending = t.pending[1:]
			}
			// Drain the ready signal if the queue is now empty; leave
			// it armed otherwise so the next Read picks up without a
			// fresh push.
			if len(t.pending) == 0 {
				select {
				case <-t.ready:
				default:
				}
			}
			t.inMu.Unlock()
			return n, nil
		}
		if t.closed {
			t.inMu.Unlock()
			return 0, io.EOF
		}
		t.inMu.Unlock()

		select {
		case <-t.ready:
		case <-t.closedCh:
		}
	}
}

// Write captures bytes into the screen buffer so a future assert
// command can inspect them. Always succeeds; the buffer grows
// unbounded (scenario tests run short and kill clients on teardown).
func (t *syntheticTerminal) Write(p []byte) (int, error) {
	t.mu.Lock()
	t.screen = append(t.screen, p...)
	t.mu.Unlock()
	return len(p), nil
}

// Size returns the dimensions passed to newSyntheticTerminal. Never
// mutated after construction so no lock is needed.
func (t *syntheticTerminal) Size() (int, int, error) {
	return t.cols, t.rows, nil
}

// Resize returns a channel that never fires for the lifetime of the
// terminal. close() closes it so client.Run's resize goroutine
// returns on teardown. Returning a nil channel would park the
// goroutine forever on shutdown; a closed channel is the correct
// terminator.
func (t *syntheticTerminal) Resize() <-chan tty.ResizeEvent {
	return t.resizeCh
}

// push enqueues bytes for the next Read. Returns an error wrapping
// cmd.ErrStaleClient once the terminal has been closed (the client
// exited), matching the Kill contract.
func (t *syntheticTerminal) push(b []byte) error {
	t.inMu.Lock()
	if t.closed {
		t.inMu.Unlock()
		return cmd.ErrStaleClient
	}
	// Copy: caller may reuse its slice.
	chunk := append([]byte(nil), b...)
	t.pending = append(t.pending, chunk)
	select {
	case t.ready <- struct{}{}:
	default:
	}
	t.inMu.Unlock()
	return nil
}

// close wakes any in-flight Read with io.EOF and stops further
// pushes. The closed flag under inMu makes this idempotent: calling
// it twice (e.g. from Kill + server shutdown) is safe.
func (t *syntheticTerminal) close() {
	t.inMu.Lock()
	if t.closed {
		t.inMu.Unlock()
		return
	}
	t.closed = true
	close(t.closedCh)
	close(t.resizeCh)
	t.inMu.Unlock()
}

// Screen returns a snapshot of bytes written to the terminal. Copied
// so callers can inspect without racing with concurrent Write calls.
// Satisfies cmd.ClientManager.Screen; the assert command reads from
// here via the narrow interface.
func (m *clientManager) Screen(ref string) ([]byte, error) {
	m.mu.Lock()
	sc, ok := m.clients[ref]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("server: screen for client %s: %w", ref, cmd.ErrStaleClient)
	}
	sc.term.mu.Lock()
	defer sc.term.mu.Unlock()
	return append([]byte(nil), sc.term.screen...), nil
}

// Ensure cmd.ClientManager stays satisfied by *clientManager — a
// compile-time check equivalent to a blank-import-and-assign.
var _ cmd.ClientManager = (*clientManager)(nil)
