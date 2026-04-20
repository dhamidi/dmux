package client

import (
	"fmt"
	"io"
	"os"

	"github.com/dhamidi/dmux/internal/term"
)

// Config holds the injected dependencies and settings for the dmux client.
// All I/O is explicit so the client is testable without a real terminal.
type Config struct {
	// SocketPath is the path to the server's Unix-domain socket.
	SocketPath string

	// In is the source of raw terminal input bytes (typically os.Stdin or a TTY).
	In io.Reader

	// Out is the destination for terminal output written by the server
	// (typically os.Stdout or the same TTY).
	Out io.Writer

	// Size queries the current terminal dimensions. Use [term.OSSize] for
	// a real terminal; supply a stub in tests.
	Size term.SizeFunc

	// RawMode enters raw mode on the controlling terminal and returns a
	// restore function. Use [term.OSRawMode] for a real terminal; may be
	// nil in tests.
	RawMode term.RawModeFunc

	// Argv is the command to send to the server as MSG_COMMAND, if any.
	Argv []string

	// ControlMode enables machine-readable control-mode output (-CC flag).
	ControlMode bool

	// ReadOnly prevents the client from sending stdin input to the server.
	ReadOnly bool
}

// NewOSConfig returns a Config wired to the process's real terminal.
// f is the controlling terminal file (typically os.Stdin or /dev/tty);
// out is the output writer (typically os.Stdout).
func NewOSConfig(socketPath string, f *os.File, out io.Writer) Config {
	return Config{
		SocketPath: socketPath,
		In:         f,
		Out:        out,
		Size:       term.OSSize(f),
		RawMode:    term.OSRawMode(f),
	}
}

// Run connects to the dmux server and runs the client forwarding loop.
// It returns the process exit code. The terminal is put in raw mode for
// the duration of the session and restored on exit.
//
// TODO: dial cfg.SocketPath, send VERSION + IDENTIFY messages, enter the
// stdin→socket / socket→terminal forwarding loop, handle READ_*/WRITE_*
// file-RPC messages, and EXIT.
func Run(cfg Config) int {
	t, err := term.Open(term.Config{
		In:      cfg.In,
		Out:     cfg.Out,
		Size:    cfg.Size,
		RawMode: cfg.RawMode,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "dmux: %v\n", err)
		return 1
	}
	defer t.Close()

	// TODO: implement the full client loop.
	_ = t
	return 0
}
