package script

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/xio"
)

// Dialer opens a fresh connection to a dmux server. RunLine calls
// the Dialer once per command, mirroring the per-line connection
// model used by the rest of the binary. Implementations typically
// close around a resolved socket path and a context deadline.
type Dialer func(ctx context.Context) (net.Conn, error)

// ErrCommandFailed is the sentinel returned by RunLine when the
// server answered with a non-ok CommandResult. Callers use errors.Is
// to distinguish this from transport failures (connection drops,
// socket gone) — the latter are wrapped directly without going
// through this sentinel.
var ErrCommandFailed = errors.New("script: command failed")

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
	return fmt.Sprintf("script: command %d: %s: %s", e.ID, e.Status, e.Message)
}

// Unwrap returns ErrCommandFailed so errors.Is works.
func (e *CommandError) Unwrap() error { return ErrCommandFailed }

// RunLine ships one argv to the server through a fresh connection
// produced by dial, reads the CommandResult, and returns. The
// connection closes on return; we deliberately do not wait for an
// Exit frame. For command-only commands this matches the existing
// runCommand contract; for attach-family commands (new-session,
// attach-session) closing the conn after CommandResult tells the
// server's pump loop to abandon the attach without our having to
// render anything.
//
// Errors:
//
//   - argv must be non-empty, otherwise RunLine returns a wrapped
//     error without dialing.
//   - Transport failures (dial, frame read/write) wrap the
//     underlying error directly.
//   - A non-ok CommandResult turns into a *CommandError that wraps
//     ErrCommandFailed.
func RunLine(ctx context.Context, dial Dialer, argv []string) error {
	if len(argv) == 0 {
		return errors.New("script: empty command line")
	}

	conn, err := dial(ctx)
	if err != nil {
		return fmt.Errorf("script: dial: %w", err)
	}
	defer conn.Close()

	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}

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
		TermEnv:         os.Getenv("TERM"),
		Env:             os.Environ(),
	}
	if err := fw.WriteFrame(ident); err != nil {
		return fmt.Errorf("script: write Identify: %w", err)
	}
	if err := fw.WriteFrame(&proto.CommandList{
		Commands: []proto.Command{{ID: 1, Argv: argv}},
	}); err != nil {
		return fmt.Errorf("script: write CommandList: %w", err)
	}

	for {
		f, err := fr.ReadFrame()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return errors.New("script: connection closed before CommandResult")
			}
			return fmt.Errorf("script: read frame: %w", err)
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

// RunOptions tunes script execution. Source is shown in error
// messages so users can map "script: line 7" back to a file path or
// "<stdin>" without parsing the rest of the chain.
type RunOptions struct {
	Source string
}

// Run reads commands from r line by line and dispatches each through
// RunLine. Blank lines and lines whose first non-whitespace
// character is `#` are silently skipped — Tokenize already filters
// those by returning a nil slice. The first non-Ok result aborts
// the script; subsequent lines are not executed.
//
// Errors are wrapped with the source identifier and 1-based line
// number ("<source>:7: <inner>") so the chain points at the
// offending line without losing the underlying sentinel.
func Run(ctx context.Context, dial Dialer, r io.Reader, opts RunOptions) error {
	source := opts.Source
	if source == "" {
		source = "<script>"
	}

	scanner := bufio.NewScanner(r)
	// Some scripts paste long quoted strings in for `client at`. The
	// default 64KiB buffer is fine for nearly anything a human types
	// but skimps on machine-generated input; a 1 MiB ceiling matches
	// what the rest of the binary tolerates without inviting a
	// denial-of-service.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		argv, err := Tokenize(scanner.Text())
		if err != nil {
			return fmt.Errorf("%s:%d: %w", source, lineNum, err)
		}
		if len(argv) == 0 {
			continue
		}
		if err := RunLine(ctx, dial, argv); err != nil {
			return fmt.Errorf("%s:%d: %w", source, lineNum, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%s: read: %w", source, err)
	}
	return nil
}
