package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmdq"
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/log"
	"github.com/dhamidi/dmux/internal/options"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/pty"
	"github.com/dhamidi/dmux/internal/record"
	"github.com/dhamidi/dmux/internal/session"
	"github.com/dhamidi/dmux/internal/socket"
	"github.com/dhamidi/dmux/internal/status"
	"github.com/dhamidi/dmux/internal/termcaps"
	"github.com/dhamidi/dmux/internal/termin"
	"github.com/dhamidi/dmux/internal/termout"
	"github.com/dhamidi/dmux/internal/vt"
	"github.com/dhamidi/dmux/internal/xio"
)

// Current scope (M1 walking skeleton):
//
//   - Accept loop. The server binds the socket once, creates one shared
//     vt.Runtime and one serverState, then loops on Accept spawning a
//     goroutine per connection. On ctx cancellation (kill-server) the
//     listener is closed so Accept unblocks; Run waits for every
//     per-client goroutine to drain before returning.
//   - Multiple attach clients share one pane. The first attach spawns
//     the shell at its tty dimensions; subsequent attaches reuse the
//     same pane and see it at the original size. Each attach runs its
//     own pump driven by a pane.Subscribe() dirty-signal channel, so N
//     clients render independently off a single vt.Terminal.
//   - Command-only clients (e.g. "dmux kill-server") share the Accept
//     loop but never take a subscription and never spawn a pane.
//     kill-server acks StatusOk, sets serverState.shutdownReason to
//     ExitServerExit, cancels the server ctx, and returns — every
//     attach pump observes ctx.Done and writes its own Exit frame.
//   - One session, one window, one pane, threaded through
//     internal/session. The first attach creates session "dmux",
//     adds a window named after the shell's argv[0], spawns the
//     pane, and sets it as the window's active pane. Subsequent
//     attaches reuse the same objects via the registry. No options
//     layer yet; cwd / env / shell come from the server process.
//   - doc.go still describes the full event-loop design with a main
//     goroutine, cmd registry, and session registry. This file is the
//     walking-skeleton stub. Search for TODO(m1:server-*) for the
//     replacement points.

// Run is the M1 walking-skeleton server entry point. It binds the
// AF_UNIX socket at path, loops on Accept, and runs one goroutine per
// connection under a shared context. Run returns when the context is
// canceled (kill-server) and every per-connection goroutine has
// finished, or when the initial bind/runtime setup fails.
func Run(path string) error {
	l, err := socket.Listen(path)
	if err != nil {
		return fmt.Errorf("server: listen: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// One Runtime per server process: compiling the wasm module is
	// expensive, and each Terminal gets its own Module instance anyway
	// so the runtime is safe to share across panes.
	rt, err := vt.NewRuntime(ctx)
	if err != nil {
		l.Close()
		return fmt.Errorf("server: vt runtime: %w", err)
	}
	defer rt.Close(ctx)

	// Recorder lifecycle. The recorder is process-global: when a test
	// harness has already opened one (e.g. dmuxtest driving multiple
	// scenarios through the same binary), ErrAlreadyOpen means we are
	// sharing theirs and must not Close on exit. Any other error is
	// terminal. Replay buffer size is read from the server option so
	// scenarios can subscribe to events emitted by earlier commands.
	serverOptions := options.NewServerOptions()
	replayBuf := serverOptions.GetNumber("recorded-event-buffer-size")
	recOpened := false
	if err := record.Open(record.Config{ReplayBufferSize: int(replayBuf)}); err != nil {
		if !errors.Is(err, record.ErrAlreadyOpen) {
			l.Close()
			return fmt.Errorf("server: record open: %w", err)
		}
	} else {
		recOpened = true
	}
	if recOpened {
		defer record.Close()
	}

	state := &serverState{
		ctx:            ctx,
		cancel:         cancel,
		rt:             rt,
		registry:       session.NewRegistry(),
		serverOptions:  serverOptions,
		serverSessions: make(map[session.ID]*serverSession),
		attached:       make(map[uint64]*attachedClient),
	}
	state.clients = newClientManager(state)
	state.installDefaultKeyTables()

	// Closer goroutine: when ctx is canceled (kill-server or Run's
	// defer), close the listener so the Accept loop unblocks. Without
	// this the Accept call would park forever.
	listenerClosed := make(chan struct{})
	go func() {
		<-ctx.Done()
		_ = l.Close()
		close(listenerClosed)
	}()

	var connWG sync.WaitGroup
	for {
		conn, err := l.Accept()
		if err != nil {
			// Accept returns an error once Close is called. That is the
			// only clean exit path; every other error is also terminal
			// because we have no way to rebind the socket.
			if ctx.Err() != nil {
				break
			}
			// Unexpected Accept error: stop accepting new clients and
			// let existing ones drain.
			cancel()
			break
		}
		connWG.Add(1)
		go func(c net.Conn) {
			defer connWG.Done()
			defer c.Close()
			if err := handle(c, state); err != nil {
				// The server process has nowhere to log yet — stderr is
				// /dev/null on the detached child. Per-connection errors
				// are surfaced to the client via Exit frames in handle;
				// swallowing here is intentional.
				_ = err
			}
		}(conn)
	}

	// Make sure the listener goroutine has exited before returning so
	// the socket file is gone by the time the caller observes Run's
	// return value.
	<-listenerClosed
	connWG.Wait()

	// Every client goroutine has drained — safe to tear the pane
	// (and its vt.Terminal) down now, after the final pumps have
	// already returned.
	state.shutdownRegistry()
	return nil
}

// serverState is the per-server shared state threaded through every
// per-connection goroutine: the server-wide ctx + cancel, the wasm
// runtime, the session registry, the per-session metadata
// (serverSessions) that carries each session's own ctx + exit
// bookkeeping, and the kill-server shutdown-reason handoff read by
// every pump on state.ctx cancellation.
//
// Session lifecycle is scoped: each session has its own ctx, cancelled
// by watchPaneExit when its shell exits. One session's shell ending no
// longer tears the whole server down — only the pumps attached to
// that session see their sessCtx fire and emit Exit{ExitedShell}.
// kill-server still cancels state.ctx to end every pump at once.
type serverState struct {
	ctx    context.Context
	cancel context.CancelFunc
	rt     *vt.Runtime

	// registry owns the session / window / pane object graph. Its
	// methods are NOT safe for concurrent use (see
	// internal/session); registryMu below protects every access from
	// more than one connection goroutine (creation, resolution,
	// teardown, and retirement on shell exit).
	registry *session.Registry

	// serverOptions is the root of the options parent-chain. Every
	// session's Options is parented here so Get walks local → session
	// → server → Table default. Guarded by registryMu for the same
	// reason registry is: only the main-owned goroutine mutates.
	// M1 never mutates it (no set-option command yet), but reads
	// happen on every session create / attach / render, so it sits
	// next to the registry. M5's .dmux.conf load writes here.
	serverOptions *options.Options

	// serverSessions pairs each live registry session with its
	// server-side metadata (ctx, cancel, exit reason/message, pane
	// pointer for cleanup). Keyed by the registry's session.ID. A
	// session is "live" iff it appears here; watchPaneExit removes
	// the entry when its shell exits. Guarded by registryMu.
	serverSessions map[session.ID]*serverSession

	// retiredPanes collects panes whose sessions were removed from
	// the registry by watchPaneExit. Run.shutdownRegistry closes
	// them on server exit so readLoop goroutines don't leak. Guarded
	// by registryMu.
	retiredPanes []*pane.Pane

	// registryMu guards registry, serverSessions, and retiredPanes.
	// Held only across the create / resolve / retire / teardown
	// steps; pumps do not hold it during their select loop.
	registryMu sync.Mutex

	// attachedMu guards attached and nextAttachID. Held briefly across
	// every attach/resize/detach to apply the window-size=latest
	// policy: the pane's dimensions track whichever client most
	// recently attached or sent a Resize frame.
	attachedMu   sync.Mutex
	attached     map[uint64]*attachedClient
	nextAttachID uint64

	// shutdownMu guards shutdownReason and shutdownMessage. Written
	// exactly once by kill-server and read by every pump after
	// state.ctx cancellation. A sync.Once on shutdown-set keeps
	// first-writer-wins honest.
	shutdownMu      sync.Mutex
	shutdownOnce    sync.Once
	shutdownReason  proto.ExitReason
	shutdownMessage string

	// clients is the in-process client manager handed out from every
	// serverItem's Clients() accessor. Initialized in Run after the
	// serverState literal so it can back-reference state; torn down
	// in shutdownRegistry so no syntheticClient goroutine outlives
	// Run's return.
	clients *clientManager

	// rootTable is the key table every pump starts in. Built once at
	// server startup with the default tmux-like bindings (prefix
	// "C-b" → switch to "prefix" table) and handed out read-only to
	// every pump via pumpArgs.state.
	rootTable *keys.Table
	// keyTables indexes the server's named key tables by name. The
	// "root" and "prefix" entries are populated in Run; future
	// configuration paths (.dmux.conf, bind-key) will mutate this
	// map from the main goroutine before any pump looks at it. Since
	// M1 populates the map at startup and treats it as read-only
	// thereafter, no mutex protects it — adding one belongs with the
	// first write path.
	keyTables map[string]*keys.Table
}

// serverSession is the server-side companion to one session.Session.
// It owns the session's own cancellable context (derived from nothing
// — independent of state.ctx so kill-server and shell-exit arms stay
// distinguishable in pump's select) and the exit reason/message
// recorded by watchWindow when the last window's shell dies.
type serverSession struct {
	id     session.ID
	ctx    context.Context
	cancel context.CancelFunc

	// winSubsMu guards winSubs. Taken on subscribe / unsubscribe
	// (pump attach / detach) and on every notify (background window
	// watcher). All access is quick.
	winSubsMu sync.Mutex
	// winSubs is the set of per-pump notification channels. A
	// window-list change from watchWindow fans out a non-blocking
	// send to each; coalescing per channel (buffer cap 1). One
	// channel per pump so every attached client rebinds, rather
	// than only one winning a shared signal.
	winSubs []chan struct{}

	// exitMu guards exitReason and exitMessage. Written once by
	// watchWindow before cancelling the session ctx.
	exitMu      sync.Mutex
	exitReason  proto.ExitReason
	exitMessage string
}

// subscribeWindowChanges registers a notification channel for one
// attached pump. The returned channel receives a struct{}{} whenever
// watchWindow removes a window from this session (coalescing: at
// most one pending signal per subscriber). unsubscribe must be
// called on pump teardown so the session does not hold onto a dead
// pump's channel.
func (ss *serverSession) subscribeWindowChanges() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	ss.winSubsMu.Lock()
	ss.winSubs = append(ss.winSubs, ch)
	ss.winSubsMu.Unlock()
	unsubscribe := func() {
		ss.winSubsMu.Lock()
		defer ss.winSubsMu.Unlock()
		for i, c := range ss.winSubs {
			if c == ch {
				ss.winSubs = append(ss.winSubs[:i], ss.winSubs[i+1:]...)
				return
			}
		}
	}
	return ch, unsubscribe
}

// notifyWindowChanged wakes every pump attached to this session so it
// re-resolves sess.CurrentWindow. Non-blocking per subscriber: if a
// subscriber has a pending signal, the new notify is dropped for
// that subscriber — pumps always observe the latest state when they
// do run.
func (ss *serverSession) notifyWindowChanged() {
	ss.winSubsMu.Lock()
	defer ss.winSubsMu.Unlock()
	for _, ch := range ss.winSubs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// setExit records the session's exit reason/message. Called once by
// watchPaneExit before cancelling the session's ctx.
func (ss *serverSession) setExit(reason proto.ExitReason, msg string) {
	ss.exitMu.Lock()
	ss.exitReason = reason
	ss.exitMessage = msg
	ss.exitMu.Unlock()
}

// exit reports the previously recorded exit reason/message, or zero
// values if the session ended via kill-server (watchPaneExit never
// ran because state.ctx teardown raced ahead).
func (ss *serverSession) exit() (proto.ExitReason, string) {
	ss.exitMu.Lock()
	defer ss.exitMu.Unlock()
	return ss.exitReason, ss.exitMessage
}

// attachedClient is one live attach pump's recorded tty size. Used by
// the latest-policy applier (see register / resizeAttached) to tell the
// pane what dimensions to take. The pane's actual size is the only
// source of truth for grid layout; this map exists only so we can keep
// running totals as clients come and go.
//
// TODO(m1:server-window-size-options): when internal/options lands,
// honor session.window-size = largest|smallest|manual in addition to
// the current latest policy. The struct is the right shape for any of
// them — only the chooser changes.
type attachedClient struct {
	cols int
	rows int
}

// createSession spawns a new session backed by its own window and
// pane. Pane geometry depends on the session's `status` option: with
// status on the pane takes rows-1 (the final row is the status bar);
// with status off it takes the full rows. If name is empty the server
// picks the first free numeric name ("0", "1", ...) to match tmux's
// auto-naming convention; explicit names fail fast on duplicate.
//
// clientEnv is the attaching client's own environment (from Identify);
// termEnv is the client's $TERM. createSession resolves default-shell
// and default-terminal off the session's Options before calling
// pane.Open so a future set-option landing in M2 takes effect on the
// next new-session automatically.
//
// Must be called with registryMu held. Also registers the session's
// server-side companion (serverSession) and starts its shell-exit
// watcher before returning so the lifecycle is observed from birth.
// On any failure after pane.Open, the pane is closed and the
// registry/serverSessions state is rolled back.
func (s *serverState) createSession(name string, cwd string, clientEnv []string, termEnv string, cols, rows int) (*session.Session, error) {
	if name == "" {
		name = s.autogenSessionName()
	}
	if cols <= 0 {
		cols = 80
	}
	if rows < 2 {
		rows = 24
	}

	sess, err := s.registry.NewSession(name, s.serverOptions)
	if err != nil {
		return nil, fmt.Errorf("server: new session: %w", err)
	}

	opts := sess.Options()
	argv := shellArgvFor(opts)
	env := childEnv(opts, clientEnv, termEnv)

	paneRows := rows - 1
	if !opts.GetBool("status") {
		paneRows = rows
	}

	p, err := pane.Open(s.ctx, pane.Config{
		Argv: argv,
		Cwd:  cwd,
		Env:  env,
		Cols: cols,
		Rows: paneRows,
		VT:   s.rt,
	})
	if err != nil {
		s.registry.RemoveSession(sess.ID())
		return nil, fmt.Errorf("server: open pane: %w", err)
	}

	w, err := sess.AppendWindow(filepath.Base(argv[0]))
	if err != nil {
		_ = p.Close()
		s.registry.RemoveSession(sess.ID())
		return nil, fmt.Errorf("server: append window: %w", err)
	}
	w.SetActivePane(p)

	// serverSession.ctx is independent of state.ctx: kill-server
	// cancels state.ctx, shell-exit cancels sessCtx, and pump's
	// select distinguishes the two to emit the right Exit reason.
	ssCtx, ssCancel := context.WithCancel(context.Background())
	ss := &serverSession{
		id:     sess.ID(),
		ctx:    ssCtx,
		cancel: ssCancel,
	}
	s.serverSessions[sess.ID()] = ss

	go s.watchWindow(ss, sess, w, p)

	return sess, nil
}

// autogenSessionName returns the first unused numeric name ("0",
// "1", ...). Caller must hold registryMu. Matches tmux's default
// session-naming scheme.
func (s *serverState) autogenSessionName() string {
	for i := 0; ; i++ {
		name := strconv.Itoa(i)
		if s.registry.FindSessionByName(name) == nil {
			return name
		}
	}
}

// spawnWindowInSession appends a fresh window to sess backed by its
// own pane, mirroring the session-creation path in createSession. An
// empty name defaults to the shell's argv[0] basename — matching
// tmux's default-window-name behaviour. Pane geometry mirrors
// createSession: status option decides whether the pane gives up
// the final row.
//
// Must be called with registryMu held. On failure after pane.Open,
// the pane is closed and any appended window is left on the session
// only if AppendWindow succeeded (AppendWindow itself never errors).
// A successful return leaves the new window as sess's current
// window because AppendWindow advances the cursor.
func (s *serverState) spawnWindowInSession(sess *session.Session, name, cwd string, clientEnv []string, termEnv string, cols, rows int) (*session.Window, error) {
	if cols <= 0 {
		cols = 80
	}
	if rows < 2 {
		rows = 24
	}

	opts := sess.Options()
	argv := shellArgvFor(opts)
	env := childEnv(opts, clientEnv, termEnv)

	paneRows := rows - 1
	if !opts.GetBool("status") {
		paneRows = rows
	}

	p, err := pane.Open(s.ctx, pane.Config{
		Argv: argv,
		Cwd:  cwd,
		Env:  env,
		Cols: cols,
		Rows: paneRows,
		VT:   s.rt,
	})
	if err != nil {
		return nil, fmt.Errorf("server: open pane: %w", err)
	}

	if name == "" {
		name = filepath.Base(argv[0])
	}
	w, err := sess.AppendWindow(name)
	if err != nil {
		_ = p.Close()
		return nil, fmt.Errorf("server: append window: %w", err)
	}
	w.SetActivePane(p)

	// Start a per-window exit watcher so the window closes when its
	// shell dies, matching tmux's one-pane-per-window behavior. The
	// watcher removes the window from the session when the pane
	// exits; if it was the last surviving window, the whole session
	// retires the same way the initial window would.
	ss, ok := s.serverSessions[sess.ID()]
	if !ok {
		// Should not happen: spawnWindowInSession is only called
		// against sessions created via createSession, which always
		// registers a serverSession. Defend anyway.
		_ = p.Close()
		return nil, fmt.Errorf("server: spawn window: %w", cmd.ErrNotFound)
	}
	go s.watchWindow(ss, sess, w, p)
	return w, nil
}

// paneRowsFor returns how many pane rows fit under a client tty of
// height rows given the session's `status` option. With status on the
// final tty row is reserved for the status bar; with status off the
// pane takes the whole tty.
func paneRowsFor(opts *options.Options, rows int) int {
	if !opts.GetBool("status") {
		return rows
	}
	return rows - 1
}

// register adds an attached client at (cols, rows) and applies the
// window-size=latest policy: the pane is resized so its dimensions
// match this client's tty, minus the status row when `status` is on.
// Returns an id used by resizeAttached and deregister to refer back
// to this client.
//
// The pane.Resize call signals every pump so older clients re-paint
// against the new grid dimensions. They might be smaller or larger
// than the new pane size; smaller ttys see the pane content wrap
// (TODO(m1:server-pane-clip)), larger ones see padding around the
// pane.
func (s *serverState) register(p *pane.Pane, opts *options.Options, cols, rows int) (uint64, error) {
	s.attachedMu.Lock()
	defer s.attachedMu.Unlock()

	id := s.nextAttachID
	s.nextAttachID++
	s.attached[id] = &attachedClient{cols: cols, rows: rows}

	if cols <= 0 || rows < 2 {
		return id, nil
	}
	if err := p.Resize(cols, paneRowsFor(opts, rows)); err != nil {
		return id, fmt.Errorf("server: resize on attach: %w", err)
	}
	return id, nil
}

// resizeAttached updates the recorded size for id and re-applies the
// latest policy. A Resize frame from the most recent client just
// re-sizes the pane to the same dims; from an older client it makes
// that one the new latest. Either way the pane and every pump's next
// frame catch up to the requested size.
func (s *serverState) resizeAttached(p *pane.Pane, opts *options.Options, id uint64, cols, rows int) error {
	s.attachedMu.Lock()
	defer s.attachedMu.Unlock()

	c, ok := s.attached[id]
	if !ok {
		return nil
	}
	c.cols, c.rows = cols, rows

	if cols <= 0 || rows < 2 {
		return nil
	}
	if err := p.Resize(cols, paneRowsFor(opts, rows)); err != nil {
		return fmt.Errorf("server: resize on client resize: %w", err)
	}
	return nil
}

// deregister drops id from the attached map. The latest policy does
// NOT roll back to a prior client's size on detach — the pane stays
// at whatever the most recent attach/resize set it to, and the next
// event will move it again.
func (s *serverState) deregister(id uint64) {
	s.attachedMu.Lock()
	defer s.attachedMu.Unlock()
	delete(s.attached, id)
}

// watchWindow blocks on one window's pane Exited channel and, when
// the child goes away, removes the window from the session. If other
// windows survive, attached pumps are notified so they re-resolve
// CurrentWindow (and swap their active pane if they were pinned on
// the removed one). If the removed window was the last, the session
// itself retires: exit reason/message are recorded and the session's
// ctx is cancelled so every pump attached to it observes shell-exit.
// Runs exactly once per window; ends when the pane closes.
//
// The pane IS closed here rather than parked in retiredPanes: its
// child is already gone and there's no other subscriber; holding the
// vt/pty resources open until server shutdown would leak one per
// retired window. Pumps whose pinned pane is the one closing observe
// sub.Ch closing and fall back to waiting on winChanged / ctx.Done
// on the next select iteration.
func (s *serverState) watchWindow(ss *serverSession, sess *session.Session, w *session.Window, p *pane.Pane) {
	st, ok := <-p.Exited()
	if !ok {
		// Pane was closed externally (Run teardown). Nothing to
		// announce — that path records its own shutdown reason.
		return
	}

	s.registryMu.Lock()
	sess.RemoveWindow(w)
	lastWindow := sess.CurrentWindow() == nil
	if lastWindow {
		s.registry.RemoveSession(ss.id)
		delete(s.serverSessions, ss.id)
	}
	s.registryMu.Unlock()

	if lastWindow {
		ss.setExit(proto.ExitExitedShell, exitMessage(st))
		ss.cancel()
	} else {
		ss.notifyWindowChanged()
	}

	// Release the pane's pty and vt resources. Idempotent; safe
	// alongside any Close that fires from the Run-teardown path.
	_ = p.Close()
}

// installDefaultKeyTables populates the server's root and prefix key
// tables with the M1 default bindings: "C-b" in root switches to
// the prefix table; "n" / "p" / "c" in prefix dispatch next-window /
// previous-window / new-window, respectively. Called once from Run
// after serverState is constructed but before any client handles.
// The resulting tables are handed to every pump read-only.
func (s *serverState) installDefaultKeyTables() {
	root := keys.NewTable("root")
	prefix := keys.NewTable("prefix")

	root.Bind(&keys.Binding{
		Key:         keys.KeyCode{Key: keys.KeyB, Mods: keys.ModCtrl},
		SwitchTable: "prefix",
		Note:        "switch to prefix table",
	})

	prefix.Bind(&keys.Binding{
		Key:  keys.KeyCode{Key: keys.KeyN},
		Argv: []string{"next-window"},
		Note: "select next window",
	})
	prefix.Bind(&keys.Binding{
		Key:  keys.KeyCode{Key: keys.KeyP},
		Argv: []string{"previous-window"},
		Note: "select previous window",
	})
	prefix.Bind(&keys.Binding{
		Key:  keys.KeyCode{Key: keys.KeyC},
		Argv: []string{"new-window"},
		Note: "create a new window",
	})

	s.rootTable = root
	s.keyTables = map[string]*keys.Table{
		"root":   root,
		"prefix": prefix,
	}
}

// setShutdown records why the server is going away. First writer
// wins: kill-server and shell-exit can both race here, and callers
// learn which won by reading shutdownReason under the mutex. Later
// writes are silently discarded.
func (s *serverState) setShutdown(reason proto.ExitReason, msg string) {
	s.shutdownOnce.Do(func() {
		s.shutdownMu.Lock()
		s.shutdownReason = reason
		s.shutdownMessage = msg
		s.shutdownMu.Unlock()
	})
}

// shutdown reports the reason/message the server is shutting down
// with, as previously set by setShutdown. Returns a zero reason +
// empty message if nobody has recorded anything — the generic
// server-shutting-down fallback in the pump.
func (s *serverState) shutdown() (proto.ExitReason, string) {
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()
	return s.shutdownReason, s.shutdownMessage
}

// shutdownRegistry walks every still-live session's active pane and
// every retired pane (those whose session ended via shell exit but
// whose readLoop goroutine we haven't cleaned up yet) and closes
// them. Called from Run's defer path after every per-connection
// goroutine has drained, so there are no concurrent readers or
// writers racing the close.
//
// Synthetic clients spawned via the ClientManager are torn down
// first: their server-end handle() goroutines hold pane
// subscriptions and must return before we close their panes. Kill
// waits up to killWait per client for client.Run to drain, which
// also lets the paired handle() goroutine exit on state.ctx
// cancellation.
func (s *serverState) shutdownRegistry() {
	if s.clients != nil {
		s.clients.shutdown()
	}
	s.registryMu.Lock()
	defer s.registryMu.Unlock()
	for sess := range s.registry.Sessions() {
		for _, w := range sess.Windows() {
			if p := w.ActivePane(); p != nil {
				_ = p.Close()
			}
		}
	}
	for _, p := range s.retiredPanes {
		_ = p.Close()
	}
	s.retiredPanes = nil
}

// serverItem is the cmd.Item implementation handed to every
// Exec call in a single connection's drain. It carries:
//
//   - ctx: the per-connection context, cancelled when the client
//     goes away, so command work bails with the client.
//   - ident: the Identify frame the client sent at handshake, used
//     by Client() and by session-creating commands to pull cwd,
//     env, and tty size.
//   - state: the server-wide registry and runtime. Commands reach
//     it only through the Sessions() facade.
//   - shutdown: set by Shutdown() so handle() can tell kill-server
//     ran inside this drain.
//   - attachTarget: set by SetAttachTarget from an attach-family
//     command; handle() consults it after the drain to decide
//     whether to enter pump and which session to pump.
//
// Kept private to the server package so callers outside can only
// see the narrow cmd.Item interface — commands never reach into
// serverState directly.
type serverItem struct {
	state         *serverState
	ctx           context.Context
	ident         *proto.Identify
	shutdown      bool
	attachTarget  cmd.SessionRef
	detachSet     bool
	detachReason  proto.ExitReason
	detachMessage string
	// attachedSession is the session this connection is attached to
	// when the Item dispatches a command mid-session (typically from
	// a key binding inside pump). Nil during the initial handshake
	// drain — CurrentSession surfaces that as a nil SessionRef so
	// commands can distinguish "not attached yet" from a real error.
	//
	// The pump integration that populates this field lands in a
	// later subagent; until then CurrentSession always returns nil.
	attachedSession *session.Session
}

// Context returns the per-connection context.
func (i *serverItem) Context() context.Context { return i.ctx }

// Shutdown records the message on serverState (first writer wins
// through setShutdown) and flips the local bit so the caller knows
// one of its Items asked to tear the server down.
func (i *serverItem) Shutdown(message string) {
	i.shutdown = true
	i.state.setShutdown(proto.ExitServerExit, message)
}

// Client returns this connection's client identity as seen by
// commands. The underlying Identify frame is held by value-adapter
// so commands cannot mutate it.
func (i *serverItem) Client() cmd.Client { return clientAdapter{ident: i.ident} }

// Sessions returns the registry facade. The returned value captures
// a pointer back to this Item so its Create methods know which
// client's cwd/env/geometry to use.
func (i *serverItem) Sessions() cmd.SessionLookup { return serverSessionLookup{item: i} }

// SetAttachTarget records the session this connection should attach
// to after the command queue drains. handle() reads it back to
// decide whether to enter pump and against which session.
func (i *serverItem) SetAttachTarget(ref cmd.SessionRef) { i.attachTarget = ref }

// SetDetach records the detach intent for this connection. handle()
// consults it after the queue drains: when set, it writes the
// recorded Exit frame and returns without considering attachTarget,
// so detach wins over any attach-family command in the same queue.
func (i *serverItem) SetDetach(reason proto.ExitReason, message string) {
	i.detachSet = true
	i.detachReason = reason
	i.detachMessage = message
}

// Options returns the server-scope options table. Commands that
// store cross-connection state (user options @client/<name>, hook
// bookkeeping, scriptable references) write here; inheritance
// propagates to narrower scopes.
func (i *serverItem) Options() *options.Options { return i.state.serverOptions }

// Clients returns the in-process client manager. Backed by a real
// net.Pipe()-driven synthetic-client table (see clients.go); Spawn
// creates a client that attaches to the most-recent session via the
// wire protocol, Kill tears it down, Inject feeds bytes as Input
// frames.
func (i *serverItem) Clients() cmd.ClientManager { return i.state.clients }

// CurrentSession returns this connection's attached session wrapped
// in a sessionRef, or nil when the connection is not yet attached.
// The pump-side wiring that populates attachedSession mid-session
// lands in a later subagent; during the initial handshake drain
// this always returns nil.
func (i *serverItem) CurrentSession() cmd.SessionRef {
	if i.attachedSession == nil {
		return nil
	}
	return sessionRef{sess: i.attachedSession}
}

// SpawnWindow resolves sess back to a live session.Session, opens a
// new pane under the session's default shell, and appends a window
// backed by that pane. The returned WindowRef wraps the newly
// appended window. Errors come back as fmt.Errorf %w chains so
// callers can use errors.Is/As to inspect the cause (pane.Open,
// registry mismatches, etc.).
func (i *serverItem) SpawnWindow(ref cmd.SessionRef, name string) (cmd.WindowRef, error) {
	if ref == nil {
		return nil, fmt.Errorf("server: spawn-window: %w", cmd.ErrNotFound)
	}
	state := i.state
	client := i.Client()

	state.registryMu.Lock()
	defer state.registryMu.Unlock()

	sess := state.registry.FindSession(session.ID(ref.ID()))
	if sess == nil {
		return nil, fmt.Errorf("server: spawn-window: %w", cmd.ErrNotFound)
	}
	w, err := state.spawnWindowInSession(
		sess,
		name,
		chooseCwd(client.Cwd()),
		client.Env(),
		client.TermEnv(),
		client.Cols(),
		client.Rows(),
	)
	if err != nil {
		return nil, err
	}
	return windowRef{win: w}, nil
}

// AdvanceWindow shifts sess's current-window cursor by delta,
// calling NextWindow (delta > 0) or PreviousWindow (delta < 0) the
// requested number of times. A delta of zero returns the current
// window without moving. An empty session returns an error wrapping
// ErrNotFound — the session has no windows to advance through.
func (i *serverItem) AdvanceWindow(ref cmd.SessionRef, delta int) (cmd.WindowRef, error) {
	if ref == nil {
		return nil, fmt.Errorf("server: advance-window: %w", cmd.ErrNotFound)
	}
	state := i.state
	state.registryMu.Lock()
	defer state.registryMu.Unlock()

	sess := state.registry.FindSession(session.ID(ref.ID()))
	if sess == nil {
		return nil, fmt.Errorf("server: advance-window: %w", cmd.ErrNotFound)
	}
	if sess.CurrentWindow() == nil {
		return nil, fmt.Errorf("server: advance-window: %w", cmd.ErrNotFound)
	}
	var w *session.Window
	switch {
	case delta == 0:
		w = sess.CurrentWindow()
	case delta > 0:
		for n := 0; n < delta; n++ {
			w = sess.NextWindow()
		}
	case delta < 0:
		for n := 0; n < -delta; n++ {
			w = sess.PreviousWindow()
		}
	}
	if w == nil {
		return nil, fmt.Errorf("server: advance-window: %w", cmd.ErrNotFound)
	}
	return windowRef{win: w}, nil
}

// shutdownRequested is the read side of the local bit set by
// Shutdown. Separate from serverState.shutdown() because we only
// want "did one of my Items ask?" — racing kill-servers from other
// connections should not redirect this handler down the shutdown
// path.
func (i *serverItem) shutdownRequested() bool { return i.shutdown }

// clientAdapter wraps an *proto.Identify to satisfy cmd.Client.
// Value-type on purpose so serverItem.Client hands out cheap copies;
// no mutation plumbing is needed.
type clientAdapter struct {
	ident *proto.Identify
}

func (c clientAdapter) Cwd() string     { return c.ident.Cwd }
func (c clientAdapter) Env() []string   { return c.ident.Env }
func (c clientAdapter) TermEnv() string { return c.ident.TermEnv }
func (c clientAdapter) Cols() int       { return int(c.ident.InitialCols) }
func (c clientAdapter) Rows() int       { return int(c.ident.InitialRows) }

// serverSessionLookup implements cmd.SessionLookup. It captures the
// calling Item so Create can resolve cwd / env / cols / rows from
// the attaching client without an extra parameter on the interface.
type serverSessionLookup struct {
	item *serverItem
}

// Create spawns a new session + window + pane using the calling
// client's identity for cwd, env, TERM, and initial tty dims. An
// empty name auto-generates via autogenSessionName. Returns
// ErrDuplicateSession (wrapped by session.Error) when an explicit
// name already exists.
func (l serverSessionLookup) Create(name string) (cmd.SessionRef, error) {
	state := l.item.state
	client := l.item.Client()

	state.registryMu.Lock()
	defer state.registryMu.Unlock()

	sess, err := state.createSession(
		name,
		chooseCwd(client.Cwd()),
		client.Env(),
		client.TermEnv(),
		client.Cols(),
		client.Rows(),
	)
	if err != nil {
		return nil, err
	}
	return sessionRef{sess: sess}, nil
}

// Find resolves a session by name. Returns cmd.ErrNotFound when the
// registry has no match.
func (l serverSessionLookup) Find(name string) (cmd.SessionRef, error) {
	state := l.item.state
	state.registryMu.Lock()
	defer state.registryMu.Unlock()
	sess := state.registry.FindSessionByName(name)
	if sess == nil {
		return nil, cmd.ErrNotFound
	}
	return sessionRef{sess: sess}, nil
}

// MostRecent returns the session with the highest id, or nil when
// the registry is empty. The registry iterator yields in ascending
// id order, so the last value wins.
func (l serverSessionLookup) MostRecent() cmd.SessionRef {
	state := l.item.state
	state.registryMu.Lock()
	defer state.registryMu.Unlock()
	var latest *session.Session
	for s := range state.registry.Sessions() {
		latest = s
	}
	if latest == nil {
		return nil
	}
	return sessionRef{sess: latest}
}

// List returns every session as a snapshot in ascending-id order.
func (l serverSessionLookup) List() []cmd.SessionRef {
	state := l.item.state
	state.registryMu.Lock()
	defer state.registryMu.Unlock()
	var out []cmd.SessionRef
	for s := range state.registry.Sessions() {
		out = append(out, sessionRef{sess: s})
	}
	return out
}

// sessionRef is the cmd.SessionRef implementation. It holds a
// pointer to the live session.Session so the server can resolve it
// back when entering pump; commands see only ID() and Name().
type sessionRef struct {
	sess *session.Session
}

func (r sessionRef) ID() uint64   { return uint64(r.sess.ID()) }
func (r sessionRef) Name() string { return r.sess.Name() }

// windowRef is the cmd.WindowRef implementation, shape-matched to
// sessionRef. It wraps a live session.Window so server paths that
// hand a WindowRef to command code can resolve it back when
// entering pump; commands outside the server package see only
// Index() and Name().
type windowRef struct {
	win *session.Window
}

func (r windowRef) Index() int    { return r.win.Index() }
func (r windowRef) Name() string  { return r.win.Name() }

// handle runs one client connection. Sequence:
//
//  1. Read Identify. Anything else → Exit{ProtocolError}.
//  2. Read the first CommandList. For each entry, look up Argv[0]
//     in the cmd registry and append a cmdq.Item; an unknown name
//     short-circuits with Exit{ProtocolError} before any command
//     runs.
//  3. Drain the queue, writing one CommandResult per entry. The
//     Results drive the post-drain decision:
//     - Any Item whose serverItem.Shutdown was called: write
//       Exit{<recorded reason>, <recorded msg>}, cancel server
//       ctx, return. No pane is spawned.
//     - Otherwise, if any drained command is in the attach-family
//       (attach-session or new-session) and succeeded, enter
//       handleAttach with the existing flow.
//     - Otherwise return; connection closes normally.
//  4. pump runs the render loop until the client sends Bye, the
//     connection drops, or the server ctx is canceled (kill-server
//     on another connection, or the pane's shell exited).
func handle(conn net.Conn, state *serverState) error {
	frameR := xio.NewReader(conn)
	frameW := xio.NewWriter(conn)

	ident, err := readIdentify(frameR, frameW)
	if err != nil {
		return err
	}

	first, err := frameR.ReadFrame()
	if err != nil {
		return fmt.Errorf("server: read CommandList: %w", err)
	}
	cl, ok := first.(*proto.CommandList)
	if !ok {
		_ = frameW.WriteFrame(&proto.Exit{
			Reason:  proto.ExitProtocolError,
			Message: "expected CommandList, got " + first.Type().String(),
		})
		return fmt.Errorf("server: protocol error: second frame was %s", first.Type())
	}

	// Per-connection ctx lets the command's Item.Context cancel with
	// the client going away without tearing the whole server down.
	connCtx, connCancel := context.WithCancel(state.ctx)
	defer connCancel()
	item := &serverItem{state: state, ctx: connCtx, ident: ident}

	// Build the queue. An unknown argv[0] is a protocol error — we
	// stop before executing anything so the client sees one clear
	// reason.
	var list cmdq.List
	for _, c := range cl.Commands {
		if len(c.Argv) == 0 {
			_ = frameW.WriteFrame(&proto.Exit{
				Reason:  proto.ExitProtocolError,
				Message: "empty command argv",
			})
			return fmt.Errorf("server: protocol error: empty command argv")
		}
		found, ok := cmd.Lookup(c.Argv[0])
		if !ok {
			_ = frameW.WriteFrame(&proto.Exit{
				Reason:  proto.ExitProtocolError,
				Message: "unknown command: " + c.Argv[0],
			})
			return fmt.Errorf("server: protocol error: unknown command %q", c.Argv[0])
		}
		list.Append(cmdq.Item{
			Cmd:     found,
			Argv:    c.Argv,
			CmdItem: item,
		})
	}

	results := list.Drain()

	// Emit CommandResults in order. A write failure here means the
	// client is gone; fall through to shutdown inspection so any
	// Shutdown call still takes effect.
	var writeErr error
	for i, c := range cl.Commands {
		status := proto.StatusOk
		msg := ""
		if !results[i].OK() {
			status = proto.StatusError
			msg = results[i].Error().Error()
		}
		if err := frameW.WriteFrame(&proto.CommandResult{
			ID:      c.ID,
			Status:  status,
			Message: msg,
		}); err != nil {
			writeErr = fmt.Errorf("server: write CommandResult: %w", err)
			break
		}
	}

	// A command called item.Shutdown(...): the shutdown reason is
	// already recorded on serverState. Emit our own Exit frame,
	// cancel the server ctx so every other pump wakes up, and
	// return.
	if item.shutdownRequested() {
		reason, msg := state.shutdown()
		if reason == 0 && msg == "" {
			reason = proto.ExitServerExit
		}
		if writeErr == nil {
			_ = frameW.WriteFrame(&proto.Exit{Reason: reason, Message: msg})
		}
		state.cancel()
		return writeErr
	}

	if writeErr != nil {
		return writeErr
	}

	// Detach dispatch: detach-family commands record an exit intent
	// via SetDetach. A recorded intent wins over any attach target in
	// the same queue — the connection is supposed to leave, not enter
	// pump. Emit the recorded Exit frame and return; the deferred
	// conn.Close closes the socket.
	if item.detachSet {
		_ = frameW.WriteFrame(&proto.Exit{
			Reason:  item.detachReason,
			Message: item.detachMessage,
		})
		return nil
	}

	// Attach dispatch: a successful attach-family command records a
	// target on the item via SetAttachTarget. If one landed, enter
	// pump against it; otherwise the connection closes after the
	// command drain. The check against the command name list is
	// gone — it's implicit in "did the command set a target?"
	if item.attachTarget == nil {
		return nil
	}

	return enterAttachPump(ident, conn, frameR, frameW, item)
}

// enterAttachPump is the attach-client path: resolve the session
// the attach-family command recorded on item.attachTarget, register
// this client's tty size with the latest-policy applier, subscribe
// for dirty signals, paint the initial frame, enter pump.
// CommandResults were already acked by the caller (handle) before
// this is invoked — entering pump means the queue drained
// successfully and a command set a target.
//
// Multiple attach handlers run concurrently — there is no attach
// slot to contend for. Each handler's subscription, renderer, and
// pump loop are independent; the pane is the only shared state and
// is concurrency-safe.
func enterAttachPump(
	ident *proto.Identify,
	conn net.Conn,
	frameR xio.FrameReader,
	frameW xio.FrameWriter,
	item *serverItem,
) error {
	state := item.state
	sess, ss, w, p, err := resolveAttachTarget(state, item.attachTarget)
	if err != nil {
		_ = frameW.WriteFrame(&proto.Exit{
			Reason:  proto.ExitServerExit,
			Message: err.Error(),
		})
		return err
	}
	// Populate attachedSession so commands dispatched from this
	// connection's pump (typically via a key binding) can resolve
	// CurrentSession. Safe to write here without synchronization:
	// each serverItem is owned by its own connection goroutine, and
	// no other goroutine reads this field.
	item.attachedSession = sess

	cols, rows := int(ident.InitialCols), int(ident.InitialRows)
	if cols <= 0 {
		cols = 80
	}
	if rows < 2 {
		rows = 24
	}

	// Register with the window-size=latest applier. The pane is
	// resized so this client's tty determines the session's grid
	// dimensions until another attach or Resize moves it; the rows
	// passed to the pane depend on the session's `status` option.
	// pane.Resize wakes every other pump's subscription.
	attachID, err := state.register(p, sess.Options(), cols, rows)
	if err != nil {
		_ = frameW.WriteFrame(&proto.Exit{
			Reason:  proto.ExitServerExit,
			Message: "register: " + err.Error(),
		})
		return err
	}
	defer state.deregister(attachID)

	// Renderer per client. The profile came in on Identify; the client
	// currently hard-codes Unknown (see client.handshake), which maps
	// to the least-capable feature set — safe for every real terminal.
	renderer := termout.NewRenderer(termcaps.Profile(ident.Profile))

	// Subscribe for dirty-signal wake-ups. Close on return removes
	// this subscription so the pane's readLoop stops signaling a
	// consumer that's gone.
	sub := p.Subscribe()
	defer sub.Close()

	// Drain the priming signal from Subscribe; the initial render
	// below does the job.
	select {
	case <-sub.Ch:
	default:
	}

	// Paint the initial (blank) frame so the client's tty is clean
	// before the shell's first output lands. Without this, the user
	// sees whatever was on their terminal before the attach until the
	// prompt prints.
	if err := renderAndSend(p, renderer, frameW, sess, w, cols, rows); err != nil {
		return err
	}

	return pump(pumpArgs{
		conn:     conn,
		ident:    ident,
		pane:     p,
		sub:      sub,
		frameR:   frameR,
		frameW:   frameW,
		renderer: renderer,
		sess:     sess,
		ss:       ss,
		win:      w,
		cols:     cols,
		rows:     rows,
		attachID: attachID,
		state:    state,
		item:     item,
	})
}

// resolveAttachTarget looks a SessionRef back up in the registry,
// returning the live session / serverSession / window / pane quad
// for pump use. A target that vanished between command drain and
// pump entry (or that never existed) returns an error the caller
// translates to an Exit frame. The serverSession is needed so pump
// can select on its ctx and read its exit reason on shell-exit.
func resolveAttachTarget(state *serverState, ref cmd.SessionRef) (*session.Session, *serverSession, *session.Window, *pane.Pane, error) {
	if ref == nil {
		return nil, nil, nil, nil, fmt.Errorf("server: no attach target")
	}
	state.registryMu.Lock()
	defer state.registryMu.Unlock()
	id := session.ID(ref.ID())
	sess := state.registry.FindSession(id)
	if sess == nil {
		return nil, nil, nil, nil, fmt.Errorf("server: attach target vanished: id=%d", ref.ID())
	}
	ss, ok := state.serverSessions[id]
	if !ok {
		return nil, nil, nil, nil, fmt.Errorf("server: attach target has no companion: id=%d", ref.ID())
	}
	w := sess.CurrentWindow()
	if w == nil {
		return nil, nil, nil, nil, fmt.Errorf("server: attach target has no window: id=%d", ref.ID())
	}
	p := w.ActivePane()
	if p == nil {
		return nil, nil, nil, nil, fmt.Errorf("server: attach target has no pane: id=%d", ref.ID())
	}
	return sess, ss, w, p, nil
}

// statusView builds the status.View for one client at the given tty
// width. Cols is the CLIENT's tty width (not the pane's) so the
// status bar fits the client's tty exactly — even when the pane
// dimensions differ from the client (e.g. an older client whose tty
// no longer matches the latest-policy pane size).
func statusView(sess *session.Session, curWin *session.Window, cols int) status.View {
	windows := sess.Windows()
	slots := make([]status.WindowSlot, 0, len(windows))
	for _, w := range windows {
		slots = append(slots, status.WindowSlot{
			Idx:     w.Index(),
			Name:    w.Name(),
			Current: w == curWin,
		})
	}
	return status.View{
		Session: sess.Name(),
		Windows: slots,
		Cols:    cols,
	}
}

// renderAndSend formats the pane's current screen via libghostty-vt,
// wraps the bytes with the client-specific cursor/home/erase preamble,
// and writes them as a proto.Output frame. The session's `status` and
// `status-position` options decide whether to paint the status row and
// where; when status is off no row is painted and the pane fills the
// whole tty.
//
// Both pane.Format and pane.Cursor lock against the pane's readLoop;
// the WriteFrame call happens on the pump goroutine so xio.FrameWriter's
// single-writer contract holds without extra coordination.
//
// Kitty graphics placements captured by the pane's vt parser are
// appended to the Output payload after the formatter wrap. The
// renderer drops them entirely for clients whose profile lacks
// kitty graphics support; for capable clients the first frame
// transmits image bytes and subsequent frames re-place the cached
// image ID.
//
// TODO(m1:server-render-coalesce): today we render on every pane-byte
// chunk, which is correct but wasteful — bursty output (shell prompts)
// produces several full-frame repaints when one would do. Add a small
// coalescing timer (a few ms) so consecutive chunks fold into one
// render.
//
// TODO(m1:server-status-position-top): status-position=top is read as
// a regular option today but the renderer still paints the bar at the
// bottom. Proper top support needs the formatter output shifted down
// by one row (the formatter's internal CUPs use absolute row 1 as
// home), which is a termout.Wrap change the walking skeleton does
// not attempt.
func renderAndSend(p *pane.Pane, r *termout.Renderer, w xio.FrameWriter, sess *session.Session, win *session.Window, cols, rows int) error {
	formatted, err := p.Format(r.FormatOptions())
	if err != nil {
		return fmt.Errorf("server: format: %w", err)
	}
	cur, err := p.Cursor()
	if err != nil {
		return fmt.Errorf("server: cursor: %w", err)
	}
	placements, err := p.Placements()
	if err != nil {
		return fmt.Errorf("server: placements: %w", err)
	}
	opts := sess.Options()
	var statusRow []byte
	if opts.GetBool("status") {
		statusRow = status.Render(statusView(sess, win, cols))
	}
	data := r.Wrap(formatted, cur, statusRow, rows)
	if kitty := r.EmitKitty(placements); len(kitty) > 0 {
		data = append(data, kitty...)
	}
	if err := w.WriteFrame(&proto.Output{Data: data}); err != nil {
		return fmt.Errorf("server: write Output: %w", err)
	}
	return nil
}

// readIdentify enforces the "Identify is the first frame" rule. On
// protocol violation it sends Exit{ProtocolError} best-effort and
// returns a non-nil error so the caller closes the connection.
func readIdentify(r xio.FrameReader, w xio.FrameWriter) (*proto.Identify, error) {
	f, err := r.ReadFrame()
	if err != nil {
		return nil, fmt.Errorf("server: read Identify: %w", err)
	}
	ident, ok := f.(*proto.Identify)
	if !ok {
		_ = w.WriteFrame(&proto.Exit{
			Reason:  proto.ExitProtocolError,
			Message: "expected Identify as first frame, got " + f.Type().String(),
		})
		return nil, fmt.Errorf("server: protocol error: first frame was %s", f.Type())
	}
	return ident, nil
}

// pumpArgs bundles the values pump needs. Grouped because the
// argument list grew past comfort once cols/rows/sess/win/attachID
// joined the original ctx + io plumbing.
type pumpArgs struct {
	conn     net.Conn
	ident    *proto.Identify
	pane     *pane.Pane
	sub      pane.Subscription
	frameR   xio.FrameReader
	frameW   xio.FrameWriter
	renderer *termout.Renderer
	sess     *session.Session
	ss       *serverSession
	win      *session.Window
	cols     int // initial client tty cols
	rows     int // initial client tty rows
	attachID uint64
	state    *serverState
	item     *serverItem
}

// pump is the render loop for one attached client. It owns frameW —
// every WriteFrame call for this client happens on this goroutine,
// so xio.FrameWriter's single-writer contract holds without extra
// locking. The pane's dirty-signal subscription and the socket
// reader goroutine feed this loop via channels.
//
// pump observes two independent cancellation sources:
//
//   - state.ctx: kill-server on any connection cancels it, and the
//     pump emits Exit{<recorded shutdown reason>}. All pumps end.
//   - ss.ctx: this session's shell exited, and the pump emits
//     Exit{ExitedShell, <status message>}. Only pumps attached to
//     this session end; other sessions' pumps keep running.
//
// The two ctxs are independent (ss.ctx is NOT a child of state.ctx)
// so select can distinguish them. If state.ctx fires first the
// kill-server path runs; if ss.ctx fires first the shell-exit path
// runs; if both race, whichever the scheduler picks wins and the
// other's cancellation is observed on the defer'd cleanup.
//
// Each render uses the CLIENT's current tty cols and rows (not the
// pane's) for the status bar's width and the renderer's totalRows.
// Resize frames update the recorded size and feed the latest-policy
// applier so the pane follows this client's new dimensions.
func pump(a pumpArgs) (retErr error) {
	// Per-connection ctx: cancels when the client disconnects so the
	// reader goroutine unblocks. Separate from state.ctx so one
	// client dropping does not tear the whole server down.
	ctx, cancel := context.WithCancel(a.state.ctx)
	defer cancel()

	// Pump-local mutable view of this client's tty dimensions. Resize
	// updates these in place; renderAndSend reads them every frame.
	cols := a.cols
	rows := a.rows

	// Pump-local mutable views of the attached window, pane, and
	// subscription. A window-switch binding (next-window, new-window)
	// mutates the session's current window; resyncWindow below
	// rebinds these to the new active pane and swaps the
	// subscription. Keep these distinct from pumpArgs.pane / .win /
	// .sub (which are only ever the initial values) so rebinding is
	// a local assignment.
	win := a.win
	p := a.pane
	sub := a.sub
	attachID := a.attachID

	// Subscribe to window-list change notifications. The background
	// watcher fires this whenever it removes a window from the
	// session after its pane exited; on the signal this pump calls
	// resyncWindow, which is a no-op if the change did not move the
	// current window.
	winCh, unsubscribe := a.ss.subscribeWindowChanges()
	defer unsubscribe()

	// Key-routing state. parser owns ESC / CSI / UTF-8 decoding;
	// curTable is the table the next keypress resolves against,
	// starting at root. Both live here so a mid-session window
	// switch doesn't lose the user's in-flight table position.
	parser := termin.NewParser(termcaps.Profile(a.ident.Profile))
	curTable := a.state.rootTable

	// Lazy escape timer. The parser exposes a Deadline when a lone
	// ESC has been buffered; we arm a time.Timer for that deadline
	// and rearm after every Feed/Tick. When no deadline is pending
	// the timer is stopped and its channel drained.
	//
	// Construction pattern: NewTimer with a far-future duration,
	// then Stop + drain so the initial state is "not armed". Every
	// subsequent transition is Stop + drain, optionally followed by
	// Reset. This is the canonical drain-before-reset pattern from
	// the time package docs.
	escTimer := time.NewTimer(time.Hour)
	if !escTimer.Stop() {
		<-escTimer.C
	}
	escArmed := false
	rearmEsc := func() {
		deadline, ok := parser.Deadline()
		if ok {
			if escArmed {
				if !escTimer.Stop() {
					select {
					case <-escTimer.C:
					default:
					}
				}
			}
			d := time.Until(deadline)
			if d < 0 {
				d = 0
			}
			escTimer.Reset(d)
			escArmed = true
			return
		}
		if escArmed {
			if !escTimer.Stop() {
				select {
				case <-escTimer.C:
				default:
				}
			}
			escArmed = false
		}
	}

	// resyncWindow checks whether the session's current window has
	// moved (next-window, previous-window, new-window all mutate it)
	// and, if so, swaps this pump's pinned window / pane /
	// subscription over to the new active pane so subsequent frames
	// render the right grid. Also re-points the state.register
	// attach record at the new pane so future resize-applier calls
	// land on the correct pane.
	logger := log.For("server", "session", a.sess.Name())
	resyncWindow := func() {
		newWin := a.sess.CurrentWindow()
		if newWin == nil || newWin == win {
			return
		}
		newPane := newWin.ActivePane()
		if newPane == nil {
			return
		}
		// Drop the subscription to the previous pane before taking
		// the new one — otherwise the old signals keep waking us up
		// after we've stopped caring about that pane.
		sub.Close()
		newSub := newPane.Subscribe()
		// Drain the priming signal; the initial render below does
		// the job.
		select {
		case <-newSub.Ch:
		default:
		}

		// Retire the old attachID and register the new pane so the
		// window-size=latest policy tracks the right pane from now
		// on. A Resize-during-register failure only reflects the new
		// pane's inability to resize — fall back to leaving the pane
		// at its natural size rather than bailing out of the pump.
		a.state.deregister(attachID)
		newID, err := a.state.register(newPane, a.sess.Options(), cols, rows)
		if err != nil {
			logger.Warn("resize on window switch", "err", err)
		}

		win = newWin
		p = newPane
		sub = newSub
		attachID = newID

		if err := renderAndSend(p, a.renderer, a.frameW, a.sess, win, cols, rows); err != nil {
			logger.Warn("render on window switch", "err", err)
		}
	}

	// runBinding fires one command binding and swaps the pump's
	// pane/subscription if the command moved the session's current
	// window. Closed over pump-local state so window-switching
	// commands compose without an explicit "here is the active
	// pane" plumb.
	runBinding := func(argv []string) {
		if len(argv) == 0 {
			return
		}
		found, ok := cmd.Lookup(argv[0])
		if !ok {
			logger.Warn("binding references unknown command", "argv", argv)
			return
		}
		var list cmdq.List
		list.Append(cmdq.Item{Cmd: found, Argv: argv, CmdItem: a.item})
		results := list.Drain()
		if len(results) > 0 && !results[0].OK() {
			logger.Warn("binding dispatch failed", "argv", argv, "err", results[0].Error())
		}
		resyncWindow()
	}
	dispatcher := dispatcherFunc(runBinding)

	// Reader goroutine: parse frames off the socket, deliver on
	// inCh. A single-slot readErrCh carries the terminal error
	// (io.EOF or a real failure) exactly once.
	inCh := make(chan proto.Frame, 4)
	readErrCh := make(chan error, 1)
	var readerWG sync.WaitGroup
	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		defer close(inCh)
		for {
			f, err := a.frameR.ReadFrame()
			if err != nil {
				readErrCh <- err
				return
			}
			select {
			case inCh <- f:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Shut the reader down on the way out: closing the conn unblocks
	// the reader's ReadFrame, after which readerWG can return. The
	// pane itself is NOT closed here — it is shared across clients,
	// and Run.shutdownRegistry is responsible for the final teardown.
	defer func() {
		cancel()
		_ = a.conn.Close()
		readerWG.Wait()
		sub.Close()
		if attachID != a.attachID {
			// resyncWindow swapped us onto a new attachID; the
			// enterAttachPump defer only knows about the original
			// one. Retire the current one so the window-size policy
			// doesn't keep a stale entry.
			a.state.deregister(attachID)
		}
		if !escArmed {
			return
		}
		if !escTimer.Stop() {
			select {
			case <-escTimer.C:
			default:
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			// Either state.ctx fired (kill-server on any connection)
			// or the local cancel ran in a defer. Read the recorded
			// shutdown reason and emit the right Exit category.
			reason, msg := a.state.shutdown()
			if reason == 0 && msg == "" {
				// Local-only cancel (deferred cancel with no
				// shutdown recorded yet). Fall back to the generic
				// server-shutting-down message.
				reason = proto.ExitServerExit
				msg = "server shutting down"
			}
			_ = a.frameW.WriteFrame(&proto.Exit{
				Reason:  reason,
				Message: msg,
			})
			return nil

		case <-a.ss.ctx.Done():
			// This session's shell exited. Only pumps attached to
			// this session observe this — other sessions keep
			// running. Emit the exit reason recorded by the
			// watcher; fall back to a generic ExitedShell if the
			// watcher hasn't populated it yet (race window where
			// cancel fired before setExit returned).
			reason, msg := a.ss.exit()
			if reason == 0 {
				reason = proto.ExitExitedShell
				msg = "shell ended"
			}
			_ = a.frameW.WriteFrame(&proto.Exit{
				Reason:  reason,
				Message: msg,
			})
			return nil

		case <-escTimer.C:
			escArmed = false
			emissions := parser.Tick(time.Now())
			curTable = routeInput(emissions, curTable, a.state.rootTable, a.state.keyTables, dispatcher, p)
			rearmEsc()

		case <-winCh:
			// A background watcher removed one of the session's
			// windows after its pane exited. resyncWindow swaps this
			// pump's pinned window/pane/subscription to the session's
			// new current window. A no-op if the removed window
			// wasn't this pump's current.
			resyncWindow()

		case _, ok := <-sub.Ch:
			if !ok {
				// Subscription channel closed: the pane's readLoop
				// exited (child gone). If the window watcher has
				// already moved the session to a surviving window,
				// resyncWindow rebinds us to it now; otherwise it's a
				// no-op and we wait for winChanged or ctx.Done.
				sub.Ch = nil
				resyncWindow()
				continue
			}
			// Dirty signal: either the vt.Terminal has new bytes or
			// pane.Resize fired (some other client became the latest).
			// Re-render against this client's tty dims.
			//
			// TODO(m1:server-render-coalesce): drain any pending
			// signals (non-blocking) before rendering so a burst
			// produces one frame, not N.
			if err := renderAndSend(p, a.renderer, a.frameW, a.sess, win, cols, rows); err != nil {
				return err
			}

		case f, ok := <-inCh:
			if !ok {
				// Reader is done. The error is on readErrCh; fall
				// through to read it.
				err := <-readErrCh
				if errors.Is(err, io.EOF) {
					// Client dropped without Bye. No Exit frame —
					// the socket is already gone.
					return nil
				}
				return fmt.Errorf("server: read frame: %w", err)
			}
			if rz, isResize := f.(*proto.Resize); isResize {
				// Update recorded size + apply the latest policy.
				// pane.Resize signals every pump (including this one)
				// to re-paint at the new dims.
				newCols, newRows := int(rz.Cols), int(rz.Rows)
				if newCols <= 0 || newRows < 2 {
					continue
				}
				cols, rows = newCols, newRows
				if err := a.state.resizeAttached(p, a.sess.Options(), attachID, cols, rows); err != nil {
					return err
				}
				continue
			}
			if in, isInput := f.(*proto.Input); isInput {
				// Route every byte through the parser. Key events
				// whose KeyCode resolves in the current table either
				// switch tables or fire a command; everything else
				// forwards raw bytes to the pane's pty.
				emissions := parser.Feed(in.Data)
				curTable = routeInput(emissions, curTable, a.state.rootTable, a.state.keyTables, dispatcher, p)
				rearmEsc()
				continue
			}
			if err := dispatchClientFrame(f, p, a.frameW); err != nil {
				return err
			}
			if _, isBye := f.(*proto.Bye); isBye {
				return nil
			}
		}
	}
}

// dispatchClientFrame handles one client-origin frame inside the
// pump loop. Returns a non-nil error only when the frame implies the
// connection should end (e.g. an unrecoverable write error); normal
// per-frame failures like pane.Write returning ErrClosed are treated
// as "the pane is gone, let ctx.Done handle it."
//
// proto.Input is intentionally absent from this switch: the pump
// routes Input frames through the key-binding machinery directly so
// it can mutate its local table / pane state on window-switch
// commands without round-tripping through this helper.
func dispatchClientFrame(f proto.Frame, _ *pane.Pane, w xio.FrameWriter) error {
	switch m := f.(type) {
	case *proto.CommandList:
		// Extra CommandLists after the pane is spawned: ack StatusOk
		// so the client's bookkeeping stays consistent. No-op on the
		// server side — there's still only one pane.
		// TODO(m1:server-midsession-cmd): route mid-session
		// CommandLists through the cmd registry + cmdq.List drain
		// path the same way the initial-handshake CommandList does.
		// For the walking skeleton we rubber-stamp every entry
		// because there is no other command to run once the pane is
		// up.
		for _, cmd := range m.Commands {
			if err := w.WriteFrame(&proto.CommandResult{
				ID:     cmd.ID,
				Status: proto.StatusOk,
			}); err != nil {
				return fmt.Errorf("server: write CommandResult: %w", err)
			}
		}
		return nil

	case *proto.Bye:
		if err := w.WriteFrame(&proto.Exit{Reason: proto.ExitDetached}); err != nil {
			return fmt.Errorf("server: write Exit: %w", err)
		}
		return nil

	case *proto.CapsUpdate:
		// Walking skeleton has no termcaps layer to apply this to.
		// TODO(m1:server-caps): feed into the client's termcaps
		// profile once internal/termcaps is wired in.
		return nil

	default:
		// Identify appearing twice, or any other unexpected type.
		// Fail closed — the contract is clear enough that a repeat is
		// a bug worth surfacing.
		_ = w.WriteFrame(&proto.Exit{
			Reason:  proto.ExitProtocolError,
			Message: "unexpected frame " + f.Type().String(),
		})
		return fmt.Errorf("server: unexpected frame %s", f.Type())
	}
}

// shellArgvFor returns argv for the pane child, resolved from the
// session's default-shell option. If the option is still at the Table
// default ("/bin/sh") and the server process has a non-empty $SHELL,
// prefer $SHELL — the rationale is that M1 has no .dmux.conf or
// set-option command yet, so a user's inherited $SHELL is the only
// ergonomic path to their real login shell. Once M2 lands set-option
// and M5 lands .dmux.conf, an explicit set-option -g default-shell
// wins over $SHELL (IsSetLocally on the server options would be true,
// so this fallback would not trigger).
//
// Login-shell flag is not set — M1 runs the shell as an interactive
// child under the pty; shell config loads per whatever the invoking
// shell does on a plain interactive start.
func shellArgvFor(opts *options.Options) []string {
	shell := opts.GetString("default-shell")
	if shell == "/bin/sh" {
		if envShell := os.Getenv("SHELL"); envShell != "" {
			shell = envShell
		}
	}
	return []string{shell}
}

// chooseCwd falls back to the server process's cwd when the client
// didn't send one. The client's Cwd is its own at Identify time,
// which is what tmux calls "session-creation-time cwd."
func chooseCwd(clientCwd string) string {
	if clientCwd != "" {
		return clientCwd
	}
	wd, err := os.Getwd()
	if err != nil {
		return "/"
	}
	return wd
}

// childEnv builds the env slice passed to the child shell. It starts
// from the server's own environment, drops any existing TERM, and
// picks the client's TermEnv when non-empty (so the pane believes
// it's the client's terminal type) — else falls back to the session's
// default-terminal option. The client-supplied Env is layered last so
// session-level overrides from the attaching client take effect.
func childEnv(opts *options.Options, clientEnv []string, termEnv string) []string {
	base := os.Environ()
	out := make([]string, 0, len(base)+len(clientEnv)+1)
	for _, kv := range base {
		if !strings.HasPrefix(kv, "TERM=") {
			out = append(out, kv)
		}
	}
	if termEnv != "" {
		out = append(out, "TERM="+termEnv)
	} else {
		out = append(out, "TERM="+opts.GetString("default-terminal"))
	}
	out = append(out, clientEnv...)
	return out
}

// exitMessage renders a short description of the child's exit state
// for the Exit frame's Message field. The client prints it; nothing
// parses it.
func exitMessage(st pty.ExitStatus) string {
	switch {
	case st.Exited:
		return fmt.Sprintf("shell exited (code %d)", st.Code)
	case st.Signal != 0:
		return fmt.Sprintf("shell killed by signal %d", st.Signal)
	default:
		return "shell ended"
	}
}
