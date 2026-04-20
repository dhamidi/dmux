// Package command is the command framework: registration, argument
// parsing, target resolution, dispatch, and the async command queue.
//
// # Boundary
//
// The framework lives here. The actual builtins live in
// command/builtin. Splitting them means (a) this package is testable
// with one-off fake commands, and (b) an embedder can build a
// stripped-down dmux by picking which builtins to import.
//
// # Core types
//
//	type Spec struct {
//	    Name     string
//	    Alias    []string       // e.g. "new-window" alias "neww"
//	    Args     ArgSpec        // flag + positional schema
//	    Target   TargetSpec     // what -t expects: session? pane?
//	    Run      func(*Ctx) Result
//	}
//
//	type Ctx struct {
//	    Server   Server         // interface — not *session.Server
//	    Client   ClientView     // snapshot of the requesting client
//	    Target   Target         // resolved from -t / defaults
//	    Args     ParsedArgs
//	    Queue    *Queue         // to enqueue follow-up commands
//	}
//
//	type Queue struct { ... }  // async, supports callback items
//
// # Key interfaces
//
// Every field in Ctx that previously held a concrete internal type
// is expressed as one of the following interfaces, so command handlers
// (and tests) never import package session.
//
//	type SessionStore interface {
//	    GetSession(id string) (SessionView, bool)
//	    GetSessionByName(name string) (SessionView, bool)
//	    ListSessions() []SessionView
//	}
//
//	type ClientStore interface {
//	    GetClient(id string) (ClientView, bool)
//	    ListClients() []ClientView
//	}
//
//	// Server is the combined interface command handlers receive in Ctx.
//	// *session.Server (wrapped at the server tier) satisfies it.
//	type Server interface {
//	    SessionStore
//	    ClientStore
//	}
//
// # View types
//
// View types (SessionView, WindowView, PaneView, ClientView) are
// plain-data snapshots that carry no live references. Target resolution
// reads from Server and returns a Target populated with view values.
// Builtin commands receive these snapshots in Ctx and use the Server
// interface for mutations.
//
// # Registration
//
// Register(Spec) — called from each builtin's init(). The full table
// of commands is the union of whatever sub-packages are imported. An
// embedder who doesn't want copy-mode just doesn't import
// command/builtin/copymode.
//
// # Target resolution
//
// The -t flag parser understands `session`, `session:window`,
// `session:window.pane`, `$id`, `@id`, `%id`, globs, and special
// markers like `{last}`, `{next}`, `{marked}`, `~` (last session).
// Lives in target.go so it's one place to fix target-parsing bugs.
// Resolution goes through SessionStore, so tests substitute a stub
// without a live server.
//
// # Queue semantics
//
// Commands are enqueued and run one at a time per client. A command
// can enqueue a Callback item that blocks the queue until an external
// event fires — this is how confirm-before and command-prompt pause
// execution for user input without blocking the server loop.
//
// Queue has no goroutines of its own; the caller drives it by calling
// Drain on the server event loop. All logging is injectable via
// Queue.SetLogger; the default logger discards output (no hidden
// writes to os.Stderr).
//
// # In isolation
//
// Register a fake "hello" command in a fresh Registry, dispatch a
// parsed CommandList against a stubServer that implements Server,
// assert on side effects. The builtin suite is not required to test
// the framework.
//
// # Non-goals
//
// No parsing of source text (parse). No format expansion (format).
// The framework only runs a CommandList; text-to-CommandList is the
// config loader's job (Tier 4).
package command
