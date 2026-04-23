package tty

import (
	"errors"
	"fmt"
	"strings"
)

// Current scope (M1 walking skeleton):
//
//   - Open, Raw, Restore, Size, Read, Write, Resize, Close: REAL.
//   - EnableModes: no-op stub; real implementation lands with
//     internal/termcaps. See TODO(m1) markers below.
//   - Windows: stubbed to ErrUnsupportedPlatform; GetConsoleScreenBufferInfo
//     and console control handler arrive later.
//
// When the stubbed pieces are replaced, the public API should
// not change for existing callers. EnableModes will gain a
// termcaps.Profile argument — see TODO(m1:tty-enablemodes).

// Sentinel errors. Callers use errors.Is to dispatch on category.
// The concrete error returned by this package is usually a
// *TTYError wrapping one of these, so errors.As can also pull out
// the operation that failed.
var (
	// ErrClosed is returned by Read, Write, Size, Raw, Restore, and
	// EnableModes when Close has already run.
	ErrClosed = errors.New("tty: closed")

	// ErrUnsupportedPlatform is returned by Open on platforms that
	// do not yet have a concrete implementation (currently Windows
	// in the walking skeleton).
	ErrUnsupportedPlatform = errors.New("tty: platform not supported")

	// ErrNotATerminal is returned by Open when the supplied stdin or
	// stdout fd is not a terminal (e.g. piped input, redirected
	// output). The wrapped cause is typically syscall.ENOTTY.
	ErrNotATerminal = errors.New("tty: not a terminal")

	// ErrRaw is returned when putting the terminal into raw mode
	// fails (IoctlGetTermios or IoctlSetTermios at enable time).
	ErrRaw = errors.New("tty: raw-mode failed")

	// ErrRestore is returned when restoring the saved termios after
	// Raw fails.
	ErrRestore = errors.New("tty: restore failed")

	// ErrGetSize is returned when Size cannot query TIOCGWINSZ (or
	// the platform equivalent).
	ErrGetSize = errors.New("tty: get-size failed")
)

// Op describes what the package was doing when an error arose.
// Carried on TTYError so callers can log or dispatch on the
// failing step without parsing Error().
type Op string

const (
	OpOpen        Op = "open"
	OpRaw         Op = "raw"
	OpRestore     Op = "restore"
	OpSize        Op = "size"
	OpEnableModes Op = "enable-modes"
	OpResize      Op = "resize"
	OpClose       Op = "close"
)

// TTYError is the concrete error type returned by this package.
// It wraps one of the sentinels so errors.Is still classifies the
// category, and carries the Op and free-form Detail for logs.
//
//	var te *tty.TTYError
//	if errors.As(err, &te) {
//	    // te.Op, te.Detail are available
//	}
//	if errors.Is(err, tty.ErrNotATerminal) { ... }
type TTYError struct {
	Op     Op
	Detail string
	Err    error
}

func (e *TTYError) Error() string {
	var b strings.Builder
	b.WriteString("tty: ")
	b.WriteString(string(e.Op))
	if e.Detail != "" {
		b.WriteString(": ")
		b.WriteString(e.Detail)
	}
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

func (e *TTYError) Unwrap() error { return e.Err }

// ttyErr constructs a *TTYError wrapping one of the sentinels.
// Used by the platform files to keep the returned error shape
// uniform with internal/pty's SpawnError.
func ttyErr(op Op, sentinel, cause error, format string, args ...any) error {
	var detail string
	if format != "" {
		detail = fmt.Sprintf(format, args...)
	}
	var underlying error
	switch {
	case sentinel != nil && cause != nil:
		underlying = fmt.Errorf("%w: %v", sentinel, cause)
	case sentinel != nil:
		underlying = sentinel
	case cause != nil:
		underlying = cause
	}
	return &TTYError{Op: op, Detail: detail, Err: underlying}
}

// ResizeEvent carries the new terminal cell dimensions. It is
// delivered on the channel returned by Resize. Cols is the number
// of columns, Rows the number of rows; both are strictly positive
// for a well-behaved terminal.
type ResizeEvent struct {
	Cols int
	Rows int
}
