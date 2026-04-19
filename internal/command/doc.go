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
//	    Alias    []string          // e.g. "new-window" alias "neww"
//	    Args     ArgSpec           // flag + positional schema
//	    Target   TargetSpec        // what -t expects: session? pane?
//	    Run      func(*Ctx) Result
//	}
//
//	type Ctx struct {
//	    Server *session.Server
//	    Client *session.Client   // nil for non-client-originated cmds
//	    Target Target            // resolved from -t / defaults
//	    Args   ParsedArgs
//	    Queue  *Queue            // to enqueue follow-up commands
//	}
//
//	type Queue struct { ... }    // async, supports callback items
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
// `session:window.pane`, `=exact`, globs, and special markers like
// `{last}`, `{next}`, `{marked}`, `~` (last session). Lives in
// target.go so it's one place to fix target parsing bugs.
//
// # Queue semantics
//
// Commands are enqueued and run one at a time per client. A command
// can enqueue a Callback item that blocks the queue until an external
// event fires — this is how confirm-before and command-prompt pause
// execution for user input without blocking the server loop.
//
// # In isolation
//
// Register a fake "hello" command, dispatch a parsed CommandList
// against a blank Server, assert on side effects. The builtin suite
// is not required to test the framework.
//
// # Non-goals
//
// No parsing of source text (parse). No format expansion (format).
// The framework only runs a CommandList; text-to-CommandList is the
// config loader's job (Tier 4).
package command
