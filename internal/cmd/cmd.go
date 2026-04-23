package cmd

import (
	"context"
	"errors"
	"sync"
)

// Command is the minimal interface every registered command
// implements. Exec takes raw argv (no flag parsing yet) and returns a
// Result that the queue collects. TODO(m1:cmd-args): add a
// stdlib-flag-based Args type here once M2 commands (new-window,
// split-window) need real flag parsing; for the three M1 commands all
// flags are deferred so argv passes through verbatim.
type Command interface {
	// Name is the canonical command name used for registry lookup.
	Name() string
	// Exec runs the command against item and returns its Result.
	// argv is the full argv of the invocation including argv[0].
	Exec(item Item, argv []string) Result
}

// Item is what Exec receives: the handle into the server world. The
// M1 surface is intentionally narrow — only what the three M1
// commands actually call. The full doc.go Item (Sessions, Options,
// Target, Client, Message, Log, Prompt, Confirm) lands alongside the
// commands that need it in M2.
type Item interface {
	// Context cancels with the owning client or the server as a
	// whole. Exec implementations may read it to bail on long work.
	Context() context.Context
	// Shutdown asks the server to exit. The message is stored as
	// the Exit frame's Message field, so it ends up in front of the
	// user.
	Shutdown(message string)
	// HasSession reports whether the server already owns a live
	// session / pane. attach-session returns ErrNotFound when this
	// is false. M1's world has at most one session so the predicate
	// is a single-bit check; once internal/session lands this
	// becomes a target-lookup.
	HasSession() bool
}

// Result is opaque: callers inspect it through OK / Error, not by
// reading fields. Constructors Ok and Err are the only supported
// ways to build one.
type Result struct {
	err error
}

// Ok returns a successful Result.
func Ok() Result { return Result{} }

// Err returns a failed Result wrapping err. A nil err still produces
// a failed Result whose Error() returns ErrNotImplemented so callers
// that forget to supply a reason still surface a usable message.
func Err(err error) Result {
	if err == nil {
		err = ErrNotImplemented
	}
	return Result{err: err}
}

// OK reports whether the Result represents success.
func (r Result) OK() bool { return r.err == nil }

// Error returns the wrapped error, or nil on success.
func (r Result) Error() error { return r.err }

// Error sentinels shared across the cmd framework and every
// sub-package's Exec. Each represents a distinct category a caller
// might dispatch on via errors.Is. Details (which target was missing,
// which flag was invalid) land on typed wrapper structs in M2; the
// sentinels here are the stable API.
var (
	// ErrUnknownCommand is returned by Lookup when no registered
	// command matches the given name. Server uses it to emit a
	// ProtocolError Exit on dispatch of an unknown argv[0].
	ErrUnknownCommand = errors.New("cmd: unknown command")

	// ErrNotFound: the target (session, window, pane, client)
	// named on the command line did not resolve.
	ErrNotFound = errors.New("cmd: not found")

	// ErrAmbiguous: a name or prefix matched more than one target.
	ErrAmbiguous = errors.New("cmd: ambiguous")

	// ErrInvalidTarget: the -t spec was syntactically valid but
	// semantically wrong (e.g. pane id on a session lookup).
	ErrInvalidTarget = errors.New("cmd: invalid target")

	// ErrNotImplemented: the command (or a flag) is declared but
	// not wired up yet. Used as the default when Err is called
	// with a nil error.
	ErrNotImplemented = errors.New("cmd: not implemented")

	// ErrParseFailure: argv failed flag.Parse or the command-line
	// syntax parser rejected it. Reserved for M2.
	ErrParseFailure = errors.New("cmd: parse failure")
)

// registry is the global name-to-Command map populated by Register
// from each sub-package's init(). A Mutex guards it so tests that
// register commands dynamically don't race on startup; real
// registrations happen before main() returns and never see contention.
var (
	registryMu sync.RWMutex
	registry   = map[string]Command{}
)

// Register adds c to the global command registry, keyed by c.Name().
// Re-registering the same name panics — a name collision at init
// time is a program bug, not a runtime condition to paper over.
func Register(c Command) {
	registryMu.Lock()
	defer registryMu.Unlock()
	name := c.Name()
	if _, ok := registry[name]; ok {
		panic("cmd: duplicate registration: " + name)
	}
	registry[name] = c
}

// Lookup returns the Command registered under name. The second
// return is false if no such command exists; callers that need a
// typed error can check errors.Is with ErrUnknownCommand after
// converting via Lookup.
//
// TODO(m1:cmd-lookup-prefix): implement tmux's unique-prefix
// matching ("new" → "new-session") and ambiguity detection here.
// M1 only does exact-name lookup.
func Lookup(name string) (Command, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	c, ok := registry[name]
	return c, ok
}
