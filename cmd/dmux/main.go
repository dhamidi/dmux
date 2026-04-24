package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/dhamidi/dmux/internal/client"
	// Blank imports populate the cmd registry at process start. This is
	// the single point where the binary decides which commands exist;
	// the server side looks everything up by name through cmd.Lookup.
	_ "github.com/dhamidi/dmux/internal/cmd/attachsession"
	_ "github.com/dhamidi/dmux/internal/cmd/client"
	_ "github.com/dhamidi/dmux/internal/cmd/detachclient"
	_ "github.com/dhamidi/dmux/internal/cmd/killserver"
	_ "github.com/dhamidi/dmux/internal/cmd/newsession"
	_ "github.com/dhamidi/dmux/internal/cmd/recorder"
	"github.com/dhamidi/dmux/internal/platform"
	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/server"
	"github.com/dhamidi/dmux/internal/socket"
	"github.com/dhamidi/dmux/internal/sockpath"
	"github.com/dhamidi/dmux/internal/tty"
	"github.com/dhamidi/dmux/internal/xio"
)

func main() {
	// Branch before flag parsing: the server child is re-exec'd
	// with the parent's argv, so interpreting flags on this side
	// would just mean flag.Parse() failures on values meant for
	// the client invocation.
	if path, ok := platform.IsServerChild(); ok {
		if err := server.Run(path); err != nil {
			// stderr is /dev/null in the detached child. Exit
			// with a non-zero code so at least the wait status
			// carries signal. A log file lands in a later pass.
			os.Exit(1)
		}
		return
	}

	if err := clientMain(); err != nil {
		fmt.Fprintf(os.Stderr, "dmux: %v\n", err)
		os.Exit(1)
	}
}

func clientMain() error {
	socketFlag := flag.String("S", "", "socket path (overrides -L and $DMUX)")
	labelFlag := flag.String("L", "", "socket label (default: \"default\")")
	flag.Parse()

	path, err := sockpath.Resolve(sockpath.Options{
		SocketPath: *socketFlag,
		Label:      *labelFlag,
	})
	if err != nil {
		return err
	}

	// sockpath does not create the parent dir (its doc.go calls
	// this out explicitly). The server needs it to exist before
	// bind; create it with 0700 to match sockpath's tmpdir check.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}

	conn, err := socket.DialOrStart(path, func() error {
		return platform.SpawnServer(path)
	})
	if err != nil {
		return err
	}
	defer conn.Close()

	// Split on the first non-flag argv element. "dmux" and
	// "dmux -S X" both come through with flag.NArg()==0 — those
	// hit the attach path. "dmux kill-server" and any future
	// command-only invocation land on the command-only path,
	// which must not open the tty. The server looks argv[0] up in
	// cmd.Lookup; nothing on this side parses command names.
	if flag.NArg() > 0 {
		return runCommand(conn, flag.Args())
	}
	return attach(conn)
}

// attach is the M1 walking-skeleton client-side attach path:
//
//   - Wrap stdin/stdout in a tty.TTY, put the terminal into raw mode,
//     defer Restore so the user's shell comes back usable even on
//     panic or unexpected exit.
//   - Build client.Options from the process environment (Cwd, TERM,
//     os.Environ) and a single attach-session command.
//   - Delegate to client.Run, which runs the four-goroutine byte
//     pump until the server sends Exit, the connection drops, or
//     ctx is canceled.
//   - Print a short human-readable summary of the Result before
//     returning so the user sees *why* the session ended (detached
//     vs. shell-exit vs. protocol-error).
//
// TODO(m1:cmd-signals): wire SIGINT/SIGTERM into ctx so ^C at the
// host terminal (before raw mode is established, or after Restore)
// cancels Run cleanly. In raw mode ^C goes through as a byte, so
// this only affects the narrow startup/shutdown windows.
func attach(conn net.Conn) error {
	t, err := tty.Open(os.Stdin, os.Stdout)
	if err != nil {
		return fmt.Errorf("open tty: %w", err)
	}
	// Restore first, then Close — Close already calls Restore but
	// listing both makes the intent obvious and is idempotent.
	defer t.Close()

	if err := t.Raw(); err != nil {
		return fmt.Errorf("raw: %w", err)
	}

	// Enter the alternate screen buffer so the session's frame stream
	// does not disturb the user's shell scrollback, and guarantee the
	// matching leave via defer so a panic in client.Run still hands
	// the terminal back to the user's prior state. The explicit call
	// after Run returns is what keeps the exit-summary line (printed
	// to stderr below) on the user's restored primary screen instead
	// of hidden inside the about-to-be-torn-down alt screen.
	if _, err := t.Write([]byte("\x1b[?1049h")); err != nil {
		return fmt.Errorf("enter alt screen: %w", err)
	}
	leaveAltScreen := sync.OnceFunc(func() {
		_, _ = t.Write([]byte("\x1b[?1049l"))
	})
	defer leaveAltScreen()

	opts, err := buildClientOptions()
	if err != nil {
		return err
	}

	ctx := context.Background()
	res, err := client.Run(ctx, conn, t, opts)
	// Leave the alt screen *before* Restore so the leave sequence
	// travels through the unchanged output path, then put the tty back
	// before writing anything to stderr — otherwise the error line
	// renders in the middle of a raw-mode scroll region and looks
	// like garbage.
	leaveAltScreen()
	_ = t.Restore()

	if err != nil {
		// Lost-connection errors are expected when the server exits
		// cleanly without sending Exit (rare; usually only happens
		// when the server process is killed). Surface them but
		// distinguish from protocol errors.
		if errors.Is(err, client.ErrLostConnection) {
			fmt.Fprintln(os.Stderr, "dmux: server connection lost")
			return nil
		}
		return err
	}

	printExitSummary(res)
	return nil
}

// runCommand is the command-only client path: "dmux kill-server" and
// any other non-attach invocation. It is deliberately minimal and
// does not touch the tty — no raw mode, no alt screen. The server
// either acks and tears down (kill-server) or answers with a
// command's StatusOk/StatusError chain and then Exit.
//
// Sequence:
//
//  1. Send Identify with InitialCols=InitialRows=0. Zeroes signal
//     "not an attach client" to a future server that cares; today's
//     server ignores them outside the attach path.
//  2. Send CommandList{argv}.
//  3. Read frames until Exit: each CommandResult is printed to
//     stderr on non-ok status, CapsUpdate and Beep are ignored.
//  4. On Exit, print a summary identical to the attach path so the
//     user sees why the server is departing.
//
// Returns a non-nil error on transport failure or unexpected frame;
// in that case main() exits non-zero. A clean Exit{ServerExit} from
// the server returns nil even though it terminates the process the
// user was interacting with — that is the intended outcome of
// kill-server.
func runCommand(conn net.Conn, argv []string) error {
	cwd, _ := os.Getwd()
	ident := &proto.Identify{
		ProtocolVersion: proto.ProtocolVersion,
		Profile:         0,
		InitialCols:     0,
		InitialRows:     0,
		Cwd:             cwd,
		TTYName:         "",
		TermEnv:         os.Getenv("TERM"),
		Env:             os.Environ(),
	}
	cmds := []proto.Command{{ID: 1, Argv: argv}}

	fw := xio.NewWriter(conn)
	fr := xio.NewReader(conn)

	if err := fw.WriteFrame(ident); err != nil {
		return fmt.Errorf("write Identify: %w", err)
	}
	if err := fw.WriteFrame(&proto.CommandList{Commands: cmds}); err != nil {
		return fmt.Errorf("write CommandList: %w", err)
	}

	for {
		f, err := fr.ReadFrame()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				// Server closed the socket without an Exit frame. For
				// kill-server the server cancels its ctx and returns,
				// and on some timings the socket tears down before
				// the Exit write lands on this side. Treat as success
				// — the caller's exit code is what matters.
				return nil
			}
			return fmt.Errorf("read frame: %w", err)
		}
		switch m := f.(type) {
		case *proto.CommandResult:
			if m.Status != proto.StatusOk {
				fmt.Fprintf(os.Stderr, "dmux: command %d: %s: %s\n",
					m.ID, m.Status, m.Message)
			}
		case *proto.Exit:
			printExitSummary(client.Result{
				ExitReason:  m.Reason,
				ExitMessage: m.Message,
			})
			return nil
		case *proto.Beep, *proto.CapsUpdate, *proto.Output:
			// Ignore: a command-only client has nowhere to render
			// these. Output in particular should not reach a
			// command-only connection, but drop it safely rather
			// than tripping the default arm.
		default:
			return fmt.Errorf("unexpected frame %s", f.Type())
		}
	}
}

// buildClientOptions captures the client's identity from the
// process environment. All fields are best-effort; the server
// tolerates empty strings and applies its own defaults (see
// internal/server.chooseCwd, .childEnv, .shellArgv).
//
// TODO(m1:cmd-caps): once internal/termcaps exists, run its profile
// probe here and pass the resulting Profile + Features through
// Options. For now Profile is 0 (Unknown) and Features is empty.
func buildClientOptions() (client.Options, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return client.Options{}, fmt.Errorf("getwd: %w", err)
	}
	return client.Options{
		Profile: 0,
		Cwd:     cwd,
		TTYName: os.Stdin.Name(),
		TermEnv: os.Getenv("TERM"),
		Env:     os.Environ(),
		// Bare `tmux` creates a new session every invocation
		// (running it from a second terminal does not auto-attach
		// to the first). Matching that: the default client command
		// is new-session.
		Commands: []proto.Command{
			{ID: 1, Argv: []string{"new-session"}},
		},
	}, nil
}

// printExitSummary writes a one-line description of how the session
// ended to stderr. The terminal has already been restored to cooked
// mode by the caller; stdout is reserved for the server-rendered
// frame stream so the user's scrollback survives.
func printExitSummary(res client.Result) {
	switch res.ExitReason {
	case proto.ExitDetached, proto.ExitDetachedOther:
		fmt.Fprintln(os.Stderr, "[detached]")
	case proto.ExitExitedShell:
		if res.ExitMessage != "" {
			fmt.Fprintf(os.Stderr, "[%s]\n", res.ExitMessage)
		} else {
			fmt.Fprintln(os.Stderr, "[exited]")
		}
	default:
		fmt.Fprintf(os.Stderr, "[%s: %s]\n", res.ExitReason, res.ExitMessage)
	}
}
