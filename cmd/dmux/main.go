package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/dhamidi/dmux/internal/client"
	"github.com/dhamidi/dmux/internal/platform"
	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/server"
	"github.com/dhamidi/dmux/internal/socket"
	"github.com/dhamidi/dmux/internal/sockpath"
	"github.com/dhamidi/dmux/internal/tty"
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

	opts, err := buildClientOptions()
	if err != nil {
		return err
	}

	ctx := context.Background()
	res, err := client.Run(ctx, conn, t, opts)
	// Put the tty back before writing anything to stderr — otherwise
	// the error line renders in the middle of a raw-mode scroll
	// region and looks like garbage.
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
		Commands: []proto.Command{
			{ID: 1, Argv: []string{"attach-session"}},
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
