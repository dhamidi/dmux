// Package cmd defines the Command interface, the command-language
// parser, the typed-arguments framework, and the registry of known
// commands.
//
// Reads from a command implementer's perspective. If you are
// writing a new command, this is the doc you need.
//
// # The Command interface
//
//	type Command interface {
//	    Name() string
//	    Aliases() []string
//	    Help() string
//	    NewArgs() Args                                 // returns a fresh argv parser
//	    Exec(item Item, args Args) Result
//	}
//
// `Args` and `Item` are interfaces (see below). Commands almost
// always register via the generic helper `cmd.New[T]` rather than
// implementing Command by hand:
//
//	type Args struct {
//	    Detached bool
//	    Name     string
//	    StartDir string
//	    Command  []string
//	    flagSet  *flag.FlagSet                        // initialized in Bind
//	}
//
//	func (a *Args) Bind(fs *flag.FlagSet) {
//	    fs.BoolVar(&a.Detached, "d", false, "don't attach")
//	    fs.StringVar(&a.Name, "s", "", "session name")
//	    fs.StringVar(&a.StartDir, "c", "", "start directory")
//	    a.flagSet = fs
//	}
//
//	func (a *Args) Positional() []string { return a.flagSet.Args() }
//
//	func init() {
//	    cmd.Register(cmd.New("new-session", []string{"new"}, exec))
//	}
//
//	func exec(item cmd.Item, a *Args) cmd.Result {
//	    if a.Detached {
//	        return cmd.Ok
//	    }
//	    return cmd.Run(attachsession.Cmd, "-t", resolved.Name)
//	}
//
// # Args via the standard flag package
//
// dmux uses Go's stdlib `flag` package for argument parsing rather
// than struct tags. Each command's Args type implements:
//
//	type Args interface {
//	    Bind(*flag.FlagSet)        // register flags on the FlagSet
//	    Positional() []string       // accessor for non-flag args
//	}
//
// The framework's job is to wire it up:
//
//  1. Call NewArgs() to get a zero-value Args.
//  2. Create a new flag.FlagSet for this command.
//  3. Call args.Bind(fs).
//  4. Call fs.Parse(argv).
//  5. Pass the populated Args (and Item) to Exec.
//
// Why stdlib flag rather than struct tags:
//
//   - It's standard. No reflection, no codegen, no novel tag dialect.
//     Every Go programmer can read it.
//   - flag.FlagSet has Visit, Lookup, and a usage formatter we get
//     for free.
//   - A typed field via flag.BoolVar is just as compile-checked as
//     a tagged struct field.
//
// Tradeoff accepted: registration is two lines (struct + Bind method)
// instead of one (struct with tags). For commands with 5-15 flags,
// this reads more clearly anyway.
//
// # Result type
//
//	type Result struct { ... }                       // opaque
//
//	cmd.Ok                                            // success
//	cmd.Err(err error)                                // failure with structured err
//	cmd.Await[T any](ch <-chan T, then func(T) Result) Result
//	cmd.Run(c Command, args ...string) Result         // chain into another command
//	cmd.RunList(list *List) Result                    // chain a parsed list
//
// # Item interface (what Exec receives)
//
// `Item` is an interface defined here in cmd because cmd is the
// consumer. The server implements it on a private `serverItem`
// struct. This breaks what would otherwise be a cmd<->server import
// cycle.
//
//	type Item interface {
//	    // Identity
//	    Context() context.Context             // cancels with the owning client
//	    Client() Client                       // may be nil for server-driven items
//
//	    // Resolved targets (after -t/-s flag interpretation)
//	    Target() Target                       // the command's target (-t)
//	    Source() Target                       // the command's source (-s)
//
//	    // World access (commands cannot import server)
//	    Sessions() SessionLookup
//	    Options() OptionLookup                // scoped to current target if any
//	    Shutdown()                            // request server exit
//
//	    // Continuation primitives
//	    Prompt(text string) <-chan PromptResult  // command-prompt builtin
//	    Confirm(text string) <-chan bool          // confirm-before builtin
//
//	    // Output for direct messaging (display-message)
//	    Message(text string)                  // sends to status-line overlay on the calling client
//
//	    // Logging that's tagged with the item context
//	    Log() *slog.Logger                    // pre-tagged with command name, item id
//	}
//
//	type Target struct {
//	    Session SessionRef                    // nil if unresolved
//	    Window  WindowRef                     // nil if no window targeted
//	    Pane    PaneRef                       // nil if no pane targeted
//	}
//
//	type PromptResult struct {
//	    Text      string
//	    Cancelled bool                        // true if user pressed Esc
//	}
//
// `SessionRef`, `WindowRef`, `PaneRef`, `SessionLookup`,
// `OptionLookup`, and `Client` are all interfaces defined in this
// package, also for the import-cycle reason. Server provides
// concrete implementations.
//
// The interface set is deliberately small — a command can read
// session/window/pane state, write to the status line, prompt the
// user, request shutdown, log, and access scoped options. It can't
// reach into server internals, can't write to other clients
// directly, can't enumerate connections. If a command needs more,
// the right move is to extend Item, not to break the boundary.
//
// # Errors
//
// All errors returned from Exec via cmd.Err must be structured —
// either sentinel values or named struct types matchable via
// errors.Is / errors.As. The shared error vocabulary lives in this
// package so commands and server consume it consistently:
//
//	var (
//	    ErrNotFound       = errors.New("not found")
//	    ErrAmbiguous      = errors.New("ambiguous")
//	    ErrInvalidTarget  = errors.New("invalid target")
//	    ErrNotImplemented = errors.New("not implemented")
//	    ErrParseFailure   = errors.New("parse failure")
//	)
//
//	type TargetError struct {
//	    Kind   TargetKind         // Session | Window | Pane | Client
//	    Spec   string              // the user-supplied -t value
//	    Reason error               // ErrNotFound | ErrAmbiguous | ErrInvalidTarget
//	}
//	func (e *TargetError) Error() string { ... }
//	func (e *TargetError) Unwrap() error { return e.Reason }
//
//	type ParseError struct {
//	    Source SourceLoc
//	    Reason error                // wrapped flag.Parse error or our own
//	}
//
// Commands return these via cmd.Err. The status-line message
// formatter (in internal/status) inspects errors via errors.As to
// render appropriate styles (red for fatal, yellow for missing
// target).
//
// # Registry
//
//	Register(Command)
//	Lookup(name string) (Command, error)             // unique-prefix match
//
// Unique-prefix matching matches tmux: "new" resolves to
// "new-session" if no other command starts with "new". Collisions
// return ErrAmbiguous wrapped with the candidate names.
//
// # Parser
//
// Three input shapes:
//
//	"new-session -d -s work"                          // single invocation
//	"kill-pane ; new-window"                          // semicolon list
//	"if-shell true { new-window }"                    // brace groups
//	"bind -N 'Create a new window' c { new-window }"  // bind-key syntax
//
// What the parser does NOT do in milestone one:
//
//   - Format-string expansion (#{...}); values pass through literally.
//     Format expansion is its own subsystem (M5).
//   - Shell interpolation.
//   - Conditionals beyond if-shell's static truth test.
//
//	Parse(text string) (*List, error)
//	ParseArgv(argv []string) (*List, error)
//
// A `*List` is a tree of Invocations with their parsed Args.
//
// # Milestone-one scope
//
// Three commands implemented:
//
//	new-session, attach-session, kill-server
//
// Plus enough of the framework to register them and the parser to
// accept their argv. M2-2 adds bind-key, unbind-key, list-keys,
// detach-client, send-prefix, command-prompt, display-message,
// new-window, kill-window, next/previous/select-window.
//
// # Scope boundary
//
// Parsing, registry, args framework, Item interface, error
// vocabulary. Execution semantics live in each command sub-package.
// Scheduling lives in internal/cmdq. Key-binding storage lives in
// internal/keys.
//
// # Corresponding tmux code
//
// tmux's cmd.c (registry, Lookup), cmd-parse.y (the grammar — dmux
// implements it by hand), and args.c (which dmux replaces with the
// stdlib flag package).
package cmd
