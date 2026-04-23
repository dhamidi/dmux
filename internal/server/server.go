package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmd/attachsession"
	"github.com/dhamidi/dmux/internal/cmd/newsession"
	"github.com/dhamidi/dmux/internal/cmdq"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/pty"
	"github.com/dhamidi/dmux/internal/session"
	"github.com/dhamidi/dmux/internal/socket"
	"github.com/dhamidi/dmux/internal/status"
	"github.com/dhamidi/dmux/internal/termcaps"
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

	state := &serverState{
		ctx:      ctx,
		cancel:   cancel,
		rt:       rt,
		registry: session.NewRegistry(),
	}

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
// per-connection goroutine. The M1 walking skeleton keeps this small:
// a shared ctx + cancel, the wasm runtime, the session registry (one
// session, one window, one pane in M1), and the shutdown-reason
// handoff used to categorize Exit frames on every attach pump at
// shutdown time.
//
// The one-and-only pane now lives behind the registry:
// registry → Session → Window → ActivePane. ensureSession creates
// them on first attach and watchPaneExit observes the pane's
// lifecycle so the shell-exit case cancels the server ctx.
type serverState struct {
	ctx    context.Context
	cancel context.CancelFunc
	rt     *vt.Runtime

	// registry owns the session / window / pane object graph. Its
	// methods are NOT safe for concurrent use (see
	// internal/session); registryMu below protects the subset the
	// server touches from more than one connection goroutine (the
	// ensureSession fast path and shutdownRegistry teardown).
	registry *session.Registry

	// registryMu guards the ensureSession fast path and the pane
	// size stored after first-attach. Held only across the create
	// step; pumps do not hold it.
	registryMu sync.Mutex
	paneCols   int
	paneRows   int

	// shutdownMu guards shutdownReason and shutdownMessage. Both are
	// written exactly once (by whichever actor initiates shutdown —
	// kill-server handler or the shell-exit watcher goroutine) and
	// read by every pump after ctx.Done fires. A sync.Once on
	// shutdown-set keeps first-writer-wins honest.
	shutdownMu      sync.Mutex
	shutdownOnce    sync.Once
	shutdownReason  proto.ExitReason
	shutdownMessage string
}

// ensureSession returns the shared session's active pane, creating
// the session + window + pane on first call. First-attach wins on
// size: whichever client calls ensureSession first fixes the pane's
// dimensions for the lifetime of the server. Every subsequent attach
// sees the pane at that size regardless of its own tty, and the pump
// silently ignores Resize frames from clients.
//
// The returned *session.Session and *session.Window are the live
// ones the caller should read names off for the status bar.
//
// TODO(m1:server-pane-resize-negotiation): replace first-attach-wins
// with tmux-style min-across-clients shrinking so a small client
// joining a session does not squeeze the larger ones, and a larger
// client joining does not leave the rest looking at blank cells.
func (s *serverState) ensureSession(ident *proto.Identify) (*session.Session, *session.Window, *pane.Pane, int, int, error) {
	s.registryMu.Lock()
	defer s.registryMu.Unlock()

	// Fast path: registry already has the one session from a prior
	// attach. Return its window's active pane at the recorded size.
	if sess := s.registry.FindSessionByName("dmux"); sess != nil {
		w := sess.CurrentWindow()
		return sess, w, w.ActivePane(), s.paneCols, s.paneRows, nil
	}

	cols, rows := int(ident.InitialCols), int(ident.InitialRows)
	if cols <= 0 {
		cols = 80
	}
	if rows < 2 {
		rows = 24
	}
	paneRows := rows - 1

	argv := shellArgv()
	p, err := pane.Open(s.ctx, pane.Config{
		Argv: argv,
		Cwd:  chooseCwd(ident.Cwd),
		Env:  childEnv(ident.Env, ident.TermEnv),
		Cols: cols,
		Rows: paneRows,
		VT:   s.rt,
	})
	if err != nil {
		return nil, nil, nil, 0, 0, fmt.Errorf("server: open pane: %w", err)
	}

	// Wire up the object graph. NewSession / AddWindow only fail on
	// duplicate-name, which cannot happen here: the fast path above
	// short-circuits when "dmux" already exists. Any error is a
	// logic bug, so wrap with %w and surface to the caller — they
	// will translate to an Exit frame.
	sess, err := s.registry.NewSession("dmux")
	if err != nil {
		_ = p.Close()
		return nil, nil, nil, 0, 0, fmt.Errorf("server: new session: %w", err)
	}
	w, err := sess.AddWindow(filepath.Base(argv[0]))
	if err != nil {
		_ = p.Close()
		s.registry.RemoveSession(sess.ID())
		return nil, nil, nil, 0, 0, fmt.Errorf("server: add window: %w", err)
	}
	w.SetActivePane(p)

	s.paneCols = cols
	s.paneRows = rows

	// Watch the pane for shell exit. When the child goes, mark the
	// shutdown reason as ExitedShell so every attach pump writes the
	// correct Exit category, then cancel the server ctx.
	go s.watchPaneExit(p)

	return sess, w, p, cols, rows, nil
}

// watchPaneExit blocks on the pane's Exited channel and, when the
// child goes away, sets the server's shutdown reason/message and
// cancels the server ctx so every attach pump's ctx.Done arm fires.
// Runs exactly once per pane lifetime; ends when the pane closes.
func (s *serverState) watchPaneExit(p *pane.Pane) {
	st, ok := <-p.Exited()
	if !ok {
		// Pane was closed before a status arrived (Close called in
		// shutdown path). Nothing to announce — whoever triggered the
		// close already set the shutdown reason.
		return
	}
	s.setShutdown(proto.ExitExitedShell, exitMessage(st))
	s.cancel()
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

// shutdownRegistry walks every session's window's active pane and
// closes it. Called from Run's defer path after every per-connection
// goroutine has drained, so there are no concurrent readers or
// writers racing the close. M1 has at most one session / one window /
// one pane, but the loop is written against the registry's iterator
// so it keeps working as the graph grows.
func (s *serverState) shutdownRegistry() {
	s.registryMu.Lock()
	defer s.registryMu.Unlock()
	for sess := range s.registry.Sessions() {
		w := sess.CurrentWindow()
		if w == nil {
			continue
		}
		if p := w.ActivePane(); p != nil {
			_ = p.Close()
		}
	}
}

// serverItem is the cmd.Item implementation handed to every
// Exec call in a single connection's drain. It carries the
// per-connection ctx (so a command's Context cancels with the
// client, not the whole server) and a pointer back to serverState
// for the pieces that genuinely live server-wide (shutdown, pane
// presence).
//
// Kept private to the server package so callers outside can only
// see the narrow cmd.Item interface — commands never reach into
// serverState directly.
type serverItem struct {
	state    *serverState
	ctx      context.Context
	shutdown bool
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

// HasSession reports whether the server owns a session to attach
// to. With the session registry in place this is a real predicate:
// attach-session on a fresh server (empty registry) returns
// cmd.ErrNotFound, matching tmux's "no sessions" behavior. The
// default client invocation is new-session (see
// cmd/dmux.buildClientOptions) so bare `dmux` still spawns the
// first pane on a fresh server.
//
// TODO(m1:server-session-target): consult the target-specific
// session once -t parsing lands. M1 checks presence, not identity.
func (i *serverItem) HasSession() bool { return i.state.registry.Len() > 0 }

// shutdownRequested is the read side of the local bit set by
// Shutdown. Separate from serverState.shutdown() because we only
// want "did one of my Items ask?" — racing kill-servers from other
// connections should not redirect this handler down the shutdown
// path.
func (i *serverItem) shutdownRequested() bool { return i.shutdown }

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
	item := &serverItem{state: state, ctx: connCtx}

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

	// Attach-family dispatch: any successful attach-session or
	// new-session transitions this connection to the render pump.
	// TODO(m1:server-attach-family-flag): replace this hardcoded
	// name list with a Command-interface flag (e.g. TakesAttach()
	// bool) once more commands land and the walking skeleton can
	// afford the interface churn.
	enterPump := false
	for i, c := range cl.Commands {
		if !results[i].OK() {
			continue
		}
		name := c.Argv[0]
		if name == attachsession.Name || name == newsession.Name {
			enterPump = true
			break
		}
	}

	if !enterPump {
		return nil
	}

	return enterAttachPump(ident, conn, frameR, frameW, state)
}

// enterAttachPump is the attach-client path: ensure the shared
// pane, subscribe for dirty signals, paint the initial frame, enter
// pump. CommandResults were already acked by the caller (handle)
// before this is invoked — entering pump means the command queue
// drained successfully and at least one attach-family command
// returned Ok.
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
	state *serverState,
) error {
	sess, w, p, cols, rows, err := state.ensureSession(ident)
	if err != nil {
		_ = frameW.WriteFrame(&proto.Exit{
			Reason:  proto.ExitServerExit,
			Message: "spawn: " + err.Error(),
		})
		return err
	}

	// Total rows includes the status line; the pane itself is rows-1
	// cells tall (see ensureSession for the initial sizing).
	totalRows := rows

	// Renderer per client. The profile came in on Identify; the client
	// currently hard-codes Unknown (see client.handshake), which maps
	// to the least-capable feature set — safe for every real terminal.
	renderer := termout.NewRenderer(termcaps.Profile(ident.Profile))

	// Status view: names come from the real session and window now.
	// Current is always true in M1 because there is exactly one
	// window in the session and the attached client is by definition
	// looking at it.
	sv := status.View{
		Session:    sess.Name(),
		WindowIdx:  w.Index(),
		WindowName: w.Name(),
		Current:    true,
		Cols:       cols,
	}

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
	if err := renderAndSend(p, renderer, frameW, sv, totalRows); err != nil {
		return err
	}

	return pump(conn, p, sub, frameR, frameW, renderer, sv, totalRows, state)
}

// renderAndSend formats the pane's current screen via libghostty-vt,
// wraps the bytes with the client-specific cursor/home/erase preamble,
// and writes them as a proto.Output frame.
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
func renderAndSend(p *pane.Pane, r *termout.Renderer, w xio.FrameWriter, sv status.View, totalRows int) error {
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
	statusRow := status.Render(sv)
	data := r.Wrap(formatted, cur, statusRow, totalRows)
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

// pump is the render loop for one attached client. It owns frameW —
// every WriteFrame call for this client happens on this goroutine,
// so xio.FrameWriter's single-writer contract holds without extra
// locking. The pane's dirty-signal subscription and the socket
// reader goroutine feed this loop via channels.
//
// pump observes the server-wide ctx (via state.ctx). When either
// kill-server on another connection or the shell-exit watcher
// cancels that ctx, pump reads state.shutdown() to pick the right
// Exit category, writes it, and returns.
func pump(
	conn net.Conn,
	p *pane.Pane,
	sub pane.Subscription,
	frameR xio.FrameReader,
	frameW xio.FrameWriter,
	renderer *termout.Renderer,
	sv status.View,
	totalRows int,
	state *serverState,
) (retErr error) {
	// Per-connection ctx: cancels when the client disconnects so the
	// reader goroutine unblocks. Separate from state.ctx so one
	// client dropping does not tear the whole server down.
	ctx, cancel := context.WithCancel(state.ctx)
	defer cancel()

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
			f, err := frameR.ReadFrame()
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
		_ = conn.Close()
		readerWG.Wait()
	}()

	for {
		select {
		case <-ctx.Done():
			// Server ctx canceled (kill-server on another connection,
			// or the pane's shell exited and the watcher canceled) —
			// look up why and emit the right Exit category to this
			// client.
			reason, msg := state.shutdown()
			if reason == 0 && msg == "" {
				// Local-only cancel (deferred cancel with no
				// shutdown recorded yet). Fall back to the generic
				// server-shutting-down message.
				reason = proto.ExitServerExit
				msg = "server shutting down"
			}
			_ = frameW.WriteFrame(&proto.Exit{
				Reason:  reason,
				Message: msg,
			})
			return nil

		case _, ok := <-sub.Ch:
			if !ok {
				// Subscription channel closed: the pane's readLoop
				// exited (child gone). The shell-exit watcher has
				// either already fired or will in a moment; wait for
				// ctx.Done on the next iteration rather than writing
				// Exit here with no shutdown reason available yet.
				sub.Ch = nil
				continue
			}
			// Dirty signal: the vt.Terminal has new bytes since we
			// last rendered. Re-render and send.
			//
			// TODO(m1:server-render-coalesce): drain any pending
			// signals (non-blocking) before rendering so a burst
			// produces one frame, not N.
			if err := renderAndSend(p, renderer, frameW, sv, totalRows); err != nil {
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
			// Resize from any attached client is ignored today. The
			// first-attach-wins size is recorded on serverState and
			// every pump renders against those dimensions; letting a
			// second client resize would violate that invariant and
			// confuse the first client. See ensureSession for the
			// TODO(m1:server-pane-resize-negotiation) pointer.
			if _, isResize := f.(*proto.Resize); isResize {
				continue
			}
			if err := dispatchClientFrame(f, p, frameW); err != nil {
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
func dispatchClientFrame(f proto.Frame, p *pane.Pane, w xio.FrameWriter) error {
	switch m := f.(type) {
	case *proto.Input:
		// Short write or ErrClosed here means the pane went away; the
		// ctx.Done arm will observe that and emit Exit on its own.
		_, _ = p.Write(m.Data)
		return nil

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

// shellArgv returns argv for the pane child. $SHELL wins when set,
// else /bin/sh. Login-shell flag is not set — M1 runs the shell as
// an interactive child under the pty; shell config will load per
// whatever the invoking shell does on plain interactive start.
// TODO(m1:server-shell): honor default-shell / default-command
// options once internal/options lands.
func shellArgv() []string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return []string{sh}
	}
	return []string{"/bin/sh"}
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
// appends TERM from the client's TermEnv (so the pane believes it's
// the client's terminal type). The client-supplied Env is layered
// last so session-level overrides from the attaching client take
// effect.
// TODO(m1:server-env): merge with the options-layered environment
// once internal/options exists.
func childEnv(clientEnv []string, termEnv string) []string {
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
		out = append(out, "TERM=xterm-256color")
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
