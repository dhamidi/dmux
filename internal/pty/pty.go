package pty

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors. Callers use errors.Is to dispatch on category.
// The concrete error returned by this package is usually a
// *SpawnError wrapping one of these, so errors.As can also pull
// out the operation that failed.
var (
	// ErrClosed is returned by Read, Write, Resize, and Signal when
	// Close has already run.
	ErrClosed = errors.New("pty: closed")

	// ErrUnsupportedPlatform is returned by Spawn on platforms that
	// do not yet have a concrete implementation (currently Windows
	// in the walking skeleton).
	ErrUnsupportedPlatform = errors.New("pty: platform not supported")

	// ErrOpenPty is returned when the kernel-side pty pair could not
	// be allocated: /dev/ptmx open, grantpt/unlockpt, slave open.
	ErrOpenPty = errors.New("pty: openpty failed")

	// ErrStartProcess is returned when the slave was allocated but
	// fork/exec of the child failed.
	ErrStartProcess = errors.New("pty: start process failed")

	// ErrResize is returned when TIOCSWINSZ (or ResizePseudoConsole
	// on Windows) fails.
	ErrResize = errors.New("pty: resize failed")

	// ErrSignal is returned when sending a signal to the child (or
	// its process group) fails.
	ErrSignal = errors.New("pty: signal failed")
)

// Op describes what the package was doing when an error arose.
// Carried on SpawnError so callers can log or dispatch on the
// failing step without parsing Error().
type Op string

const (
	OpOpenPtmx    Op = "open-ptmx"
	OpGrantPt     Op = "grantpt"
	OpUnlockPt    Op = "unlockpt"
	OpPtsName     Op = "ptsname"
	OpOpenSlave   Op = "open-slave"
	OpSetWinsize  Op = "set-winsize"
	OpStart       Op = "start-process"
	OpResize      Op = "resize"
	OpSignal      Op = "signal"
)

// SpawnError is the concrete error type returned by Spawn when the
// underlying platform call fails. It wraps one of the sentinels so
// errors.Is still classifies the category, and carries the Op and
// free-form Detail for logs.
//
//	var se *pty.SpawnError
//	if errors.As(err, &se) {
//	    // se.Op, se.Detail are available
//	}
//	if errors.Is(err, pty.ErrOpenPty) { ... }
type SpawnError struct {
	Op     Op
	Detail string
	Err    error
}

func (e *SpawnError) Error() string {
	var b strings.Builder
	b.WriteString("pty: ")
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

func (e *SpawnError) Unwrap() error { return e.Err }

// spawnErr constructs a *SpawnError wrapping one of the sentinels.
// Used by the platform files to keep the returned error shape
// uniform.
func spawnErr(op Op, sentinel, cause error, format string, args ...any) error {
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
	return &SpawnError{Op: op, Detail: detail, Err: underlying}
}

// Config describes the child process to spawn.
//
// The caller has already resolved Argv (argv[0] is the executable
// path, not a name to look up), merged Env from the appropriate
// option scopes, and picked Cwd. Cols and Rows set the pty's
// initial window size; zero values mean "leave to the kernel
// default" (typically 80x24 on Linux; effectively unset on
// Darwin).
type Config struct {
	Argv []string
	Cwd  string
	Env  []string
	Cols int
	Rows int
}

// Signal is the OS-independent subset of signals the pane
// abstraction needs to deliver to its child. The pty layer maps
// these onto the platform primitives.
//
// On Unix, Signal values correspond to SIGINT, SIGTERM, SIGKILL,
// SIGHUP, SIGQUIT and are delivered to the child's process group
// (negative pid) so that shells forwarding to their children also
// see them.
//
// On Windows, Signal is best-effort and implemented via
// GenerateConsoleCtrlEvent where possible; arbitrary signals
// return ErrSignalUnsupported.
type Signal int

const (
	SIGINT  Signal = 2
	SIGQUIT Signal = 3
	SIGKILL Signal = 9
	SIGTERM Signal = 15
	SIGHUP  Signal = 1
)

// ExitStatus describes how a child terminated.
//
// Exactly one of Code or Signal carries useful information:
//
//   - Exited == true: Code is the exit code (0..255 on Unix;
//     up to 32 bits on Windows).
//   - Exited == false: Signal is the signal that killed the
//     process (Unix only; Windows always reports Exited=true).
type ExitStatus struct {
	Exited bool
	Code   int
	Signal Signal
}
