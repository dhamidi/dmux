package cmd

import (
	"context"
	"errors"
	"sync"

	"github.com/dhamidi/dmux/internal/options"
	"github.com/dhamidi/dmux/internal/proto"
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

// Item is what Exec receives: the handle into the server world.
// Commands read the calling client's identity (cwd, env, tty size)
// through Client, look sessions up or create new ones through
// Sessions, and communicate "attach this connection to this session
// once the queue drains" through SetAttachTarget. The rest of the
// doc.go surface (Options, Target, Source, Message, Prompt, Confirm,
// Log) lands alongside the commands that need it.
type Item interface {
	// Context cancels with the owning client or the server as a
	// whole. Exec implementations may read it to bail on long work.
	Context() context.Context
	// Shutdown asks the server to exit. The message is stored as
	// the Exit frame's Message field, so it ends up in front of the
	// user.
	Shutdown(message string)
	// Client is the attach-client's view of its own identity —
	// cwd, env, TERM, initial tty dims. Commands that spawn panes
	// read from here to match the calling terminal.
	Client() Client
	// Sessions is the registry-facing slice of the world. Commands
	// use it to Create new sessions, Find existing ones by name,
	// or ask for MostRecent as the default attach target.
	Sessions() SessionLookup
	// SetAttachTarget records which session this connection should
	// attach to once the command queue drains. Attach-family
	// commands (new-session, attach-session) call this; the server
	// reads it after the queue finishes to pick the pane to pump.
	// A nil target means "no attach" — the connection closes after
	// the command drain without entering pump.
	SetAttachTarget(SessionRef)
	// SetDetach records the detach intent for this connection. After
	// the queue drains, the server reads the recorded (reason,
	// message) pair, writes proto.Exit{Reason: reason, Message:
	// message}, and closes the connection. Mutually exclusive with
	// SetAttachTarget within a single invocation — if both are set,
	// detach wins.
	SetDetach(reason proto.ExitReason, message string)
	// Options exposes the scope-chain options view this Item
	// reads from. Commands that store state scoped to the running
	// server (including user options like @client/<name>) write
	// here; inheritance propagates the value to narrower scopes.
	Options() *options.Options
	// Clients exposes the in-process client manager. Commands
	// that spawn synthetic clients (test harnesses, AI agents,
	// hooks) call Spawn to create one and Kill to tear it down.
	Clients() ClientManager
	// CurrentSession returns the session this connection is
	// currently attached to, or nil when the connection is not
	// attached. Commands triggered by a key binding typed inside an
	// attached session read the session here; commands that run
	// before attach (during the initial handshake) see nil and must
	// handle that by returning ErrNotFound or equivalent.
	CurrentSession() SessionRef
	// SpawnWindow appends a new window to sess using the server's
	// default shell, spawning the pane and wiring it as the window's
	// active pane. An empty name lets the server pick a default
	// (typically the shell basename). Returns a WindowRef for the
	// new window, or an error wrapping a server-level failure such
	// as pane spawn or pty allocation.
	SpawnWindow(sess SessionRef, name string) (WindowRef, error)
	// AdvanceWindow moves sess's current-window cursor by delta
	// (typically +1 or -1; larger magnitudes wrap multiple times).
	// Returns a WindowRef for the new current window, or an error
	// wrapping ErrNotFound when sess has no windows to advance
	// through.
	AdvanceWindow(sess SessionRef, delta int) (WindowRef, error)
}

// ClientManager is the Item-facing surface of the server's
// in-process client table. Spawn creates a client attached to the
// same server process (via the real wire protocol, not a shortcut)
// and returns an opaque reference string the caller stores for later
// lookup. Kill tears down the client named by ref; a ref that does
// not correspond to a live client returns an error wrapping
// ErrStaleClient so callers can tolerate bookkeeping drift. Inject
// writes raw bytes into the named client's input stream, so the
// server reads them as Input frames on that client's connection
// identical to keystrokes from a real tty; a ref that no longer
// corresponds to a live client returns an error wrapping
// ErrStaleClient on the same contract as Kill.
type ClientManager interface {
	Spawn(profile string, cols, rows int) (ref string, err error)
	Kill(ref string) error
	Inject(ref string, bytes []byte) error
}

// Client is the attach-client's view of its own identity. The
// server uses these fields when opening panes on the client's
// behalf (cwd, env, tty size). Kept narrow on purpose: commands
// should not reach into protocol frames directly.
type Client interface {
	// Cwd is the client process's working directory at Identify
	// time. Falls back to server process cwd when empty.
	Cwd() string
	// Env is the client-process environment, passed through to
	// panes the command spawns. Includes TERM.
	Env() []string
	// TermEnv is the client's $TERM. Separate from Env so the
	// server can overwrite the pane's TERM without scanning the
	// whole environment.
	TermEnv() string
	// Cols / Rows are the client's initial tty dimensions. Used
	// as the pane's starting geometry when Create spawns one.
	Cols() int
	Rows() int
}

// SessionLookup exposes the session registry to commands. Create
// spawns a new session (with its initial window and pane); Find
// resolves a name to a live session; MostRecent returns the
// highest-id session (the closest thing to "the default target" in
// M1); List returns every registered session. The concrete
// implementation lives in the server package.
type SessionLookup interface {
	// Create makes a new session. An empty name triggers the
	// server's auto-naming policy (numeric "0", "1", ... matching
	// tmux's default). Returns ErrDuplicateSession when an
	// explicit name already exists; the auto-naming path never
	// collides.
	Create(name string) (SessionRef, error)
	// Find returns the session with the given name, or wraps
	// ErrNotFound when none exists.
	Find(name string) (SessionRef, error)
	// MostRecent returns the session with the highest id, or nil
	// when the registry is empty. attach-session uses this as the
	// default target when no -t is given.
	MostRecent() SessionRef
	// List returns every session in ascending-id order. Used by
	// diagnostic commands (list-sessions) once they land.
	List() []SessionRef
}

// SessionRef identifies a session without exposing its internal
// object graph. The server resolves refs back to live
// session.Session instances when it needs to act on them; commands
// only read ID and Name.
type SessionRef interface {
	ID() uint64
	Name() string
}

// WindowRef identifies a window without exposing its internal
// object graph. Commands read Index and Name; the server resolves
// refs back to live session.Window instances when it needs to act
// on them. Shape-matched to SessionRef so command code that
// shuttles both looks uniform.
type WindowRef interface {
	Index() int
	Name() string
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

	// ErrStaleClient: a Kill was requested on a client reference
	// that no longer corresponds to a live client. Callers that
	// keep bookkeeping in user options (e.g. @client/<name>) use
	// errors.Is to distinguish "already gone" from real failures.
	ErrStaleClient = errors.New("cmd: stale client reference")
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
