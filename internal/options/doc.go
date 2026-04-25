// Package options is dmux's hierarchical options system.
//
// Modeled directly on tmux's options.c + options-table.c, with the
// C-isms removed. The shape is the same because the design is right:
// a closed table of known options, scoped to server / session /
// window / pane, with parent-chain inheritance for lookup.
//
// # Scopes and inheritance
//
// Four scopes:
//
//	ServerScope    one global instance
//	SessionScope   one per session
//	WindowScope    one per window
//	PaneScope      one per pane (M3+; M1/M2 don't use)
//
// Each Options instance has an optional parent pointer. Lookup walks
// the parent chain until a value is found:
//
//	pane.options.parent  ->  window.options
//	window.options.parent ->  session.options
//	session.options.parent ->  server.options
//	server.options.parent ->  nil (defaults from Table)
//
// Get(name) returns the value from the most-specific scope that has
// it set; if no scope has it, the default from the closed Table.
// Set(name, value) writes only to the scope it's called on.
//
// This is exactly tmux's options_get / options_set behaviour, just
// with Go maps instead of RB trees.
//
// # The closed Table
//
// Every known option has an entry:
//
//	type Entry struct {
//	    Name        string
//	    Aliases     []string  // tmux's "alternative_name" + array of historical aliases
//	    Type        Type      // String, Number, Bool, Key, Color, Choice, Command
//	    Scope       Scope     // bitmask: which scopes accept this option
//	    Default     Value     // typed
//	    Min, Max    int64     // for Number
//	    Choices     []string  // for Choice
//	    IsArray     bool      // repeatable, like tmux's command-alias[]
//	    Help        string
//	}
//
//	var Table = []Entry{
//	    {Name: "default-shell", Type: String, Scope: SessionScope,
//	     Default: stringValue("/bin/sh"), Help: "Shell run by new windows."},
//	    {Name: "status",        Type: Bool,   Scope: SessionScope,
//	     Default: boolValue(true), Help: "Show the status line."},
//	    {Name: "status-position", Type: Choice, Scope: SessionScope,
//	     Choices: []string{"top", "bottom"}, Default: stringValue("bottom"), ...},
//	    // ...
//	}
//
// Adding an option is one Table entry. Resist scattering option
// lookups; consult the table via Get/Set on the appropriate scope.
//
// # User options
//
// tmux allows user-defined options prefixed with `@`. We do the
// same: any option name starting with `@` bypasses the closed-table
// check and is stored as a string. This is what `set-option @my-var
// foo` plus `#{@my-var}` rely on.
//
// # Type system
//
//	type Value struct { ... }    // opaque tagged-union
//
//	String(s string) Value
//	Number(n int64) Value
//	Bool(b bool) Value
//	Key(k keys.KeyCode) Value
//	Color(c colors.RGBA) Value
//	Choice(s string) Value
//	Command(c *cmd.List) Value
//
//	(Value) AsString() (string, bool)
//	(Value) AsNumber() (int64, bool)
//	(Value) AsBool() (bool, bool)
//	// ...
//
// Typed getter helpers on Options for ergonomics:
//
//	(*Options) GetString(name string) string  // panics if Type != String
//	(*Options) GetBool(name string) bool
//	(*Options) GetNumber(name string) int64
//	// ...
//
// Panic on type mismatch is correct because the Type comes from the
// closed Table — a mismatch is a programmer error caught at the
// first call.
//
// # Interface
//
//	NewServerOptions() *Options
//	NewScopedOptions(scope Scope, parent *Options) *Options
//
//	(*Options) Get(name string) Value
//	(*Options) GetString(name string) string
//	(*Options) GetBool(name string) bool
//	(*Options) GetNumber(name string) int64
//	(*Options) Set(name string, v Value) error
//	(*Options) Unset(name string) error
//	(*Options) IsSetLocally(name string) bool
//	(*Options) Walk(yield func(name string, v Value, source Scope) bool)
//
// `Walk` is range-over-func friendly and reports which scope each
// value came from — useful for `show-options -g` / `show-options
// -A` semantics in M5.
//
// # Concurrency
//
// Options instances are safe for concurrent use. Each scope carries
// its own RWMutex; Get takes RLock per scope as it walks the parent
// chain (child-first, never re-entering a scope), Set/Unset take a
// write lock on the scope they mutate. Reads dominate; writes are
// rare (set-option), so the lock is barely contended in practice.
//
// Pane goroutines still receive snapshots of the options they care
// about at construction time (via Config when Spawn is called) so
// they don't need to reach back into Options on every render. If an
// option that affects pane behavior changes after pane spawn (M5
// territory), the server pushes a control message to the pane
// goroutine via Pane.SendOptionUpdate.
//
// # M1 scope
//
// M1 ships the Options struct, the parent-chain Get path, and the
// minimum Table entries needed for the milestone:
//
//	default-shell       (session, string)  — used by new-session
//	default-terminal    (session, string)  — set to "xterm-256color"
//	status              (session, bool)    — true in M1, drives status line
//	status-position     (session, choice)  — bottom in M1
//
// No `set-option` command in M1 — the table is read-only after
// startup. M2 adds `set-option`, `show-options`, `set-window-option`.
// `.dmux.conf` loading lands in M5.
//
// # Scope boundary
//
//   - No format-string expansion (M5; format strings reference
//     options but live in their own package).
//   - No hooks (M5; hooks ARE options of type Command, so the
//     storage path is the same, but firing them is server's job).
//   - No persistence; options live in process memory only.
//
// # Corresponding tmux code
//
// options.c (storage and lookup) and options-table.c (the closed
// Table). dmux's Table mirrors the entries we need; entries land
// over time as their consumers do.
package options
