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

	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/pty"
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
//   - One attach client at a time. A sync.Mutex-guarded flag in
//     serverState.attachInUse marks the attach slot: a second attach is
//     refused with Exit{ServerExit, "another client is attached"}. M1's
//     acceptance criteria only ask for one session, so there is no
//     pane-sharing or multi-viewer work here.
//   - Command-only clients (e.g. "dmux kill-server") share the Accept
//     loop but never take the attach slot and never spawn a pane.
//     kill-server acks StatusOk, writes Exit{ServerExit, "kill-server"},
//     cancels the server ctx, and returns — pump on any other
//     connection observes ctx.Done and tears its pane down.
//   - No cmdq, no session registry, no window, no options. Argv[0]
//     of each Command is ignored by the attach path; the first
//     CommandList on an attach connection triggers the single shell
//     pane. Any further CommandList answers StatusOk without doing
//     anything.
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
		ctx:    ctx,
		cancel: cancel,
		rt:     rt,
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
	return nil
}

// serverState is the per-server shared state threaded through every
// per-connection goroutine. The M1 walking skeleton keeps this small:
// a shared ctx + cancel, the wasm runtime, and a mutex-guarded attach
// slot. The real server will carry session/window registries, the
// command queue, and key tables here.
//
// TODO(m1:server-cmd-registry): replace the hardcoded "kill-server"
// string match in handle with a lookup into the cmd registry that
// docs/m1.md describes (internal/cmd + internal/cmd/killserver).
type serverState struct {
	ctx    context.Context
	cancel context.CancelFunc
	rt     *vt.Runtime

	// attachMu guards attachInUse. Held only briefly around the
	// take/release transitions; the pump loop does not hold it.
	attachMu     sync.Mutex
	attachInUse  bool
}

// tryTakeAttach claims the single attach slot. Returns true on success;
// false means another client already holds it.
func (s *serverState) tryTakeAttach() bool {
	s.attachMu.Lock()
	defer s.attachMu.Unlock()
	if s.attachInUse {
		return false
	}
	s.attachInUse = true
	return true
}

// releaseAttach frees the attach slot. Always paired with a successful
// tryTakeAttach; calling it when the slot is already free is a no-op so
// defers stay trivial even on error paths.
func (s *serverState) releaseAttach() {
	s.attachMu.Lock()
	defer s.attachMu.Unlock()
	s.attachInUse = false
}

// handle runs one client connection. Sequence:
//
//  1. Read Identify. Anything else → Exit{ProtocolError}.
//  2. Read the first CommandList. Dispatch by Commands[0].Argv[0]:
//     - "kill-server": ack StatusOk for every command, write
//       Exit{ServerExit, "kill-server"}, cancel the server ctx, and
//       return. No pane is spawned.
//     - anything else (attach-session, new-session, empty argv): take
//       the attach slot. If already held, write
//       Exit{ServerExit, "another client is attached"} and return.
//       Otherwise spawn the shell pane, ack commands, paint the
//       initial frame, and enter pump.
//  3. pump runs the byte-pump until the pane exits, the client sends
//     Bye, the connection drops, or the server ctx is canceled
//     (kill-server on another connection).
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

	if isKillServer(cl) {
		return handleKillServer(cl, frameW, state)
	}

	return handleAttach(ident, cl, conn, frameR, frameW, state)
}

// isKillServer returns true when the first command's argv is exactly
// ["kill-server"]. Anything else falls through to the attach path.
//
// TODO(m1:server-cmd-registry): the real server looks every command up
// in the cmd registry (internal/cmd) and dispatches through cmdq.
// Hardcoding the string here is the walking-skeleton shortcut.
func isKillServer(cl *proto.CommandList) bool {
	if len(cl.Commands) == 0 {
		return false
	}
	argv := cl.Commands[0].Argv
	return len(argv) >= 1 && argv[0] == "kill-server"
}

// handleKillServer acks every command in the list with StatusOk, writes
// a final Exit{ServerExit}, then cancels the server ctx so the Run
// loop closes the listener and any concurrent attach connection tears
// down.
func handleKillServer(cl *proto.CommandList, w xio.FrameWriter, state *serverState) error {
	for _, cmd := range cl.Commands {
		if err := w.WriteFrame(&proto.CommandResult{
			ID:     cmd.ID,
			Status: proto.StatusOk,
		}); err != nil {
			// Even if we can't tell this client, still cancel so the
			// server process exits.
			state.cancel()
			return fmt.Errorf("server: write CommandResult: %w", err)
		}
	}
	if err := w.WriteFrame(&proto.Exit{
		Reason:  proto.ExitServerExit,
		Message: "kill-server",
	}); err != nil {
		state.cancel()
		return fmt.Errorf("server: write Exit: %w", err)
	}
	state.cancel()
	return nil
}

// handleAttach is the attach-client path: take the attach slot, spawn a
// pane, ack commands, paint the initial frame, enter pump. On return,
// whether successful or not, releases the attach slot for the next
// client.
func handleAttach(
	ident *proto.Identify,
	cl *proto.CommandList,
	conn net.Conn,
	frameR xio.FrameReader,
	frameW xio.FrameWriter,
	state *serverState,
) error {
	if !state.tryTakeAttach() {
		_ = frameW.WriteFrame(&proto.Exit{
			Reason:  proto.ExitServerExit,
			Message: "another client is attached",
		})
		return nil
	}
	defer state.releaseAttach()

	// The first CommandList sets initial dimensions. If the client
	// didn't send them, fall back to 80x24 — this is the skeleton, a
	// real attach client always sends real sizes from its tty.
	//
	// The pane gets rows-1 cells of vertical space; the last row is
	// reserved for the status line that termout.Wrap paints on top of
	// the formatter output. Anything below rows=2 would leave the pane
	// with zero or negative rows, so clamp the floor.
	cols, rows := int(ident.InitialCols), int(ident.InitialRows)
	if cols <= 0 {
		cols = 80
	}
	if rows < 2 {
		rows = 24
	}
	paneRows := rows - 1
	totalRows := rows

	// Per-connection ctx is a child of the server ctx so kill-server
	// on another connection propagates down here: pump selects on
	// this ctx and closes the pane when it fires.
	ctx, cancel := context.WithCancel(state.ctx)
	defer cancel()

	p, err := pane.Open(ctx, pane.Config{
		Argv: shellArgv(),
		Cwd:  chooseCwd(ident.Cwd),
		Env:  childEnv(ident.Env, ident.TermEnv),
		Cols: cols,
		Rows: paneRows,
		VT:   state.rt,
	})
	if err != nil {
		msg := err.Error()
		_ = frameW.WriteFrame(&proto.Exit{
			Reason:  proto.ExitServerExit,
			Message: "spawn: " + msg,
		})
		return fmt.Errorf("server: open pane: %w", err)
	}

	// CommandResults go out after the pane is live so that if spawn
	// fails the client sees Exit, not a phantom StatusOk followed by
	// Exit.
	for _, cmd := range cl.Commands {
		if err := frameW.WriteFrame(&proto.CommandResult{
			ID:     cmd.ID,
			Status: proto.StatusOk,
		}); err != nil {
			cancel()
			_ = p.Close()
			return fmt.Errorf("server: write CommandResult: %w", err)
		}
	}

	// Renderer per client. The profile came in on Identify; the client
	// currently hard-codes Unknown (see client.handshake), which maps
	// to the least-capable feature set — safe for every real terminal.
	renderer := termout.NewRenderer(termcaps.Profile(ident.Profile))

	// Status view for the single-window walking skeleton. Session and
	// window names are hardcoded today — the session registry that
	// owns real names does not exist yet.
	// TODO(m1:status-session-name): replace "dmux" with the real
	// session the client attached to once internal/session lands.
	// TODO(m1:status-window-name): derive the window name from the
	// pane's command/title once internal/window lands.
	sv := status.View{
		Session:    "dmux",
		WindowIdx:  0,
		WindowName: filepath.Base(shellArgv()[0]),
		Current:    true,
		Cols:       cols,
	}

	// Paint the initial (blank) frame so the client's tty is clean
	// before the shell's first output lands. Without this, the user
	// sees whatever was on their terminal before the attach until the
	// prompt prints.
	if err := renderAndSend(p, renderer, frameW, sv, totalRows); err != nil {
		cancel()
		_ = p.Close()
		return err
	}

	return pump(ctx, cancel, conn, p, frameR, frameW, renderer, sv, totalRows)
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

// pump is the byte-pump main loop for one client + one pane. It owns
// frameW — every WriteFrame call in the server happens on this
// goroutine, so xio.FrameWriter's single-writer contract holds
// without extra locking. Reader and pane-output goroutines feed this
// loop via channels.
//
// pump also observes the server-wide ctx (threaded in via the per-
// connection ctx, which is a child of state.ctx). When kill-server on
// another connection cancels that ctx, pump writes
// Exit{ServerExit, "server shutting down"} and returns; the deferred
// teardown closes the pane.
func pump(
	ctx context.Context,
	cancel context.CancelFunc,
	conn net.Conn,
	p *pane.Pane,
	frameR xio.FrameReader,
	frameW xio.FrameWriter,
	renderer *termout.Renderer,
	sv status.View,
	totalRows int,
) (retErr error) {
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

	// Shut everything down on the way out in the order that lets
	// each goroutine observe its cue:
	//
	//  1. cancel pane context so pane.Bytes -> ctx.Done select wins
	//     if readLoop is waiting to deliver a chunk.
	//  2. Close the pane. This kills the child (SIGHUP), drains the
	//     pty, and lets pane.Close return after its helpers exit.
	//  3. Close the conn — this is what unblocks the reader
	//     goroutine's ReadFrame so readerWG.Wait can return.
	//  4. Wait for the reader.
	//
	// The caller also defers conn.Close as a safety net; the second
	// Close is a no-op error that we do not propagate.
	defer func() {
		cancel()
		if err := p.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("server: close pane: %w", err)
		}
		_ = conn.Close()
		readerWG.Wait()
	}()

	paneBytes := p.Bytes()
	paneExited := p.Exited()

	for {
		select {
		case <-ctx.Done():
			// Server ctx canceled (kill-server on another connection)
			// or this connection's ctx canceled for local reasons.
			// Announce Exit{ServerExit} so the attach client prints a
			// sensible summary, then return; defer tears the pane down.
			_ = frameW.WriteFrame(&proto.Exit{
				Reason:  proto.ExitServerExit,
				Message: "server shutting down",
			})
			return nil

		case _, ok := <-paneBytes:
			if !ok {
				// Pane reader finished. Wait for Exited (or for the
				// client to depart) via the other arms.
				paneBytes = nil
				continue
			}
			// Raw chunk is discarded — the vt.Terminal has already
			// consumed it inside pane.readLoop. The chunk's arrival is
			// just a dirty signal: format, wrap, send.
			//
			// TODO(m1:server-render-coalesce): drain additional pending
			// chunks from paneBytes (non-blocking) before rendering so
			// a burst produces one frame, not N.
			if err := renderAndSend(p, renderer, frameW, sv, totalRows); err != nil {
				return err
			}

		case st, ok := <-paneExited:
			// ExitedShell fires regardless of ok — the channel closes
			// right after the single send, so both branches mean the
			// child is gone. See pane.waitLoop.
			_ = ok
			msg := exitMessage(st)
			if err := frameW.WriteFrame(&proto.Exit{
				Reason:  proto.ExitExitedShell,
				Message: msg,
			}); err != nil {
				return fmt.Errorf("server: write Exit: %w", err)
			}
			return nil

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
			// Resize is handled inline because it mutates three pieces
			// of render state (pane rows, status cols, totalRows) that
			// must stay consistent for the next renderAndSend.
			if rs, isResize := f.(*proto.Resize); isResize {
				newCols := int(rs.Cols)
				newRows := int(rs.Rows)
				if newCols < 1 {
					newCols = 1
				}
				if newRows < 2 {
					newRows = 2
				}
				_ = p.Resize(newCols, newRows-1)
				totalRows = newRows
				sv.Cols = newCols
				if err := renderAndSend(p, renderer, frameW, sv, totalRows); err != nil {
					return err
				}
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
// as "the pane is gone, let the Exited arm handle it."
func dispatchClientFrame(f proto.Frame, p *pane.Pane, w xio.FrameWriter) error {
	switch m := f.(type) {
	case *proto.Input:
		// Short write or ErrClosed here means the pane went away; the
		// Exited arm will observe that and emit Exit on its own.
		_, _ = p.Write(m.Data)
		return nil

	case *proto.CommandList:
		// Extra CommandLists after the pane is spawned: ack StatusOk
		// so the client's bookkeeping stays consistent. No-op on the
		// server side — there's still only one pane.
		// TODO(m1:server-cmd-registry): route through the real cmd
		// registry once it exists; this is the same hardcoded path as
		// handleKillServer but for in-session CommandLists.
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
