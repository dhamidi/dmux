package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/pty"
	"github.com/dhamidi/dmux/internal/socket"
	"github.com/dhamidi/dmux/internal/xio"
)

// Current scope (M1 walking skeleton):
//
//   - Accept one client. Require Identify first. On the first
//     CommandList, spawn a single pane running $SHELL (fallback
//     /bin/sh) at the client's initial dimensions. Fan Input frames
//     into pane.Write, fan pane.Bytes() into Output frames, route
//     Resize into pane.Resize. On client Bye or pane exit, send Exit
//     and return.
//   - No cmdq, no session registry, no window, no options. Argv[0]
//     of each Command is ignored; the first CommandList triggers the
//     single shell pane. Any further CommandList answers StatusOk
//     without doing anything.
//   - doc.go still describes the full event-loop design with multiple
//     clients, panes, and sessions. This file is the walking-skeleton
//     stub. Search for TODO(m1:server-*) for the replacement points.

// Run is the M1 walking-skeleton server entry point. It binds the
// AF_UNIX socket at path, accepts exactly one client, runs a single
// shell pane over that connection, and returns when the client
// departs or the pane exits.
//
// TODO(m1:server-accept-loop): the real server loops on Accept and
// spawns per-client state. For M1 one client at a time is enough to
// exercise the full byte-pump path end-to-end.
func Run(path string) error {
	l, err := socket.Listen(path)
	if err != nil {
		return fmt.Errorf("server: listen: %w", err)
	}
	defer l.Close()

	conn, err := l.Accept()
	if err != nil {
		return fmt.Errorf("server: accept: %w", err)
	}
	// Safety-net close. pump closes the conn explicitly on the main
	// success path so that readerWG.Wait can observe the reader
	// goroutine unblocking; this defer covers error paths between
	// Accept and pump and is a no-op after an earlier Close.
	defer conn.Close()
	return handle(conn)
}

// handle runs one client. Sequence:
//
//  1. Read Identify. Anything else → Exit{ProtocolError}.
//  2. Read CommandList. Reply with StatusOk per command. The first
//     CommandList spawns the shell pane at Identify's InitialCols /
//     InitialRows. Further CommandLists are accepted but do not
//     spawn additional panes.
//  3. Enter the byte-pump: client frames (Input / Resize / Bye /
//     further CommandList) arrive on inCh from a reader goroutine;
//     pane output flows from pane.Bytes(); pane exit flows from
//     pane.Exited(). Everything funnels into frameW.WriteFrame, which
//     this goroutine owns.
//  4. On Bye → Exit{Detached}; on pane exit → Exit{ExitedShell}; on
//     client-side EOF → return without sending Exit (connection is
//     already gone).
func handle(conn net.Conn) error {
	frameR := xio.NewReader(conn)
	frameW := xio.NewWriter(conn)

	ident, err := readIdentify(frameR, frameW)
	if err != nil {
		return err
	}

	// The first CommandList sets initial dimensions. If the client
	// didn't send them, fall back to 80x24 — this is the skeleton, a
	// real client always sends real sizes from its tty.
	cols, rows := int(ident.InitialCols), int(ident.InitialRows)
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p, err := pane.Open(ctx, pane.Config{
		Argv: shellArgv(),
		Cwd:  chooseCwd(ident.Cwd),
		Env:  childEnv(ident.Env, ident.TermEnv),
		Cols: cols,
		Rows: rows,
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

	return pump(ctx, cancel, conn, p, frameR, frameW)
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
func pump(
	ctx context.Context,
	cancel context.CancelFunc,
	conn net.Conn,
	p *pane.Pane,
	frameR xio.FrameReader,
	frameW xio.FrameWriter,
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
		case chunk, ok := <-paneBytes:
			if !ok {
				// Pane reader finished. Wait for Exited (or for the
				// client to depart) via the other arms.
				paneBytes = nil
				continue
			}
			if err := frameW.WriteFrame(&proto.Output{Data: chunk}); err != nil {
				return fmt.Errorf("server: write Output: %w", err)
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

	case *proto.Resize:
		_ = p.Resize(int(m.Cols), int(m.Rows))
		return nil

	case *proto.CommandList:
		// Extra CommandLists after the pane is spawned: ack StatusOk
		// so the client's bookkeeping stays consistent. No-op on the
		// server side — there's still only one pane.
		// TODO(m1:server-cmdq): route through the real cmd registry
		// once it exists.
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
