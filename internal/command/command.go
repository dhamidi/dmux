package command

import "fmt"

// Result is the return value of a command handler.
type Result struct {
	// Err, if non-nil, means the command failed.
	Err error
	// Output is any text the command produces (e.g. for list-* commands).
	Output string
}

// OK returns a successful Result with no output.
func OK() Result { return Result{} }

// Errorf returns a failed Result wrapping a formatted error string.
func Errorf(format string, args ...any) Result {
	return Result{Err: fmt.Errorf(format, args...)}
}

// WithOutput returns a successful Result carrying text output.
func WithOutput(s string) Result { return Result{Output: s} }

// ─── View types ──────────────────────────────────────────────────────────────
//
// View types are plain-data snapshots passed between the framework and command
// handlers. They contain no live references to internal packages, so commands
// can be tested without a running server.

// SessionView is a snapshot of session state.
type SessionView struct {
	ID      string
	Name    string
	Windows []WindowView
	// Current is the index into Windows of the active window (-1 if none).
	Current int
	// LastWindowID is the ID of the window that was active before the current one.
	LastWindowID string
}

// CurrentWindow returns the active window, or a zero WindowView if none.
func (s SessionView) CurrentWindow() WindowView {
	if s.Current >= 0 && s.Current < len(s.Windows) {
		return s.Windows[s.Current]
	}
	return WindowView{}
}

// WindowView is a snapshot of window state.
type WindowView struct {
	ID    string
	Name  string
	Index int
	Panes []PaneView
	// Active is the ID of the active pane (0 if none).
	Active int
	// LastPaneID is the ID of the pane that was active before the current one (0 if none).
	LastPaneID int
	// ActivityFlag is true when unread activity or a bell has been detected.
	ActivityFlag bool
	// Cols and Rows are the dimensions of the window in character cells.
	Cols int
	Rows int
	// LastLayout is the marshalled layout string saved before the last layout
	// change; used by select-layout -o to undo.
	LastLayout string
	// CurrentPreset is the name of the last preset applied to this window.
	CurrentPreset string
	// LinkedSessions lists the IDs of all sessions this window is linked into.
	// It is empty for windows that have never been linked via link-window.
	LinkedSessions []string
}

// ActivePane returns the active pane, or a zero PaneView if none.
func (w WindowView) ActivePane() PaneView {
	for _, p := range w.Panes {
		if p.ID == w.Active {
			return p
		}
	}
	return PaneView{}
}

// PaneView is a snapshot of pane state.
type PaneView struct {
	ID    int
	Title string
}

// ClientView is a snapshot of client state. The zero value represents no
// client (non-client-originated command).
type ClientView struct {
	ID        string
	SessionID string // empty if detached
	Cols      int
	Rows      int
	TTY       string
	KeyTable  string
	// PID is the OS process ID of the dmux client process.
	PID int
}

// IsAttached reports whether the client is attached to a session.
func (c ClientView) IsAttached() bool { return c.SessionID != "" }

// ─── Store interfaces ─────────────────────────────────────────────────────────

// SessionStore is the read interface over sessions used by command handlers
// and target resolution. *session.Server (wrapped at the server tier)
// satisfies this interface.
type SessionStore interface {
	// GetSession looks up a session by its ID.
	GetSession(id string) (SessionView, bool)
	// GetSessionByName looks up a session by its display name.
	GetSessionByName(name string) (SessionView, bool)
	// ListSessions returns all sessions in an unspecified order.
	ListSessions() []SessionView
}

// ClientStore is the read interface over connected clients.
type ClientStore interface {
	// GetClient looks up a client by its ID.
	GetClient(id string) (ClientView, bool)
	// ListClients returns all connected clients.
	ListClients() []ClientView
}

// Server is the combined read interface that command handlers receive in their
// Ctx. It merges SessionStore and ClientStore so a single stub satisfies both
// for testing.
type Server interface {
	SessionStore
	ClientStore
}

// KeyBinding is a key-to-command binding entry returned by Server.ListKeyBindings.
type KeyBinding struct {
	// Table is the name of the key table, e.g. "root" or "prefix".
	Table string
	// Key is the string representation of the key, e.g. "C-b".
	Key string
	// Command is the bound command text.
	Command string
}

// OptionEntry is a name-value pair returned by Server.ListOptions.
type OptionEntry struct {
	// Name is the option name.
	Name string
	// Value is the option value as a string.
	Value string
}

// BufferEntry is a name-size pair returned by ListBuffers.
type BufferEntry struct {
	// Name is the buffer name.
	Name string
	// Size is the number of bytes in the buffer.
	Size int
}

// EnvironEntry is a name-value pair from ListEnvironment.
type EnvironEntry struct {
	// Name is the environment variable name.
	Name string
	// Value is the environment variable value.
	Value string
	// Removed is true if the variable was unset (-r flag).
	Removed bool
}

// MenuEntry is one item in a display-menu popup.
type MenuEntry struct {
	Label   string
	Key     string
	Command string
}

// ChooserItem is one selectable entry in a choose-buffer or choose-client
// interactive picker.
type ChooserItem struct {
	// Display is the line shown in the list.
	Display string
	// Preview is optional text shown in the preview pane.
	Preview string
	// Value is the %% substitution value used when an item is selected.
	Value string
}

// Mutator is the write interface that command handlers use to modify server
// state. It is a separate interface from Server so that command tests can
// stub only the write-side methods they exercise.
type Mutator interface {
	// Session mutations.
	NewSession(name string) (SessionView, error)
	KillSession(id string) error
	RenameSession(id, name string) error

	// Client mutations.
	AttachClient(clientID, sessionID string) error
	DetachClient(clientID string) error
	SwitchClient(clientID, sessionID string) error

	// Window mutations.
	NewWindow(sessionID, name string) (WindowView, error)
	KillWindow(sessionID, windowID string) error
	RenameWindow(sessionID, windowID, name string) error
	SelectWindow(sessionID, windowID string) error

	// Pane mutations.
	SplitWindow(sessionID, windowID string) (PaneView, error)
	KillPane(paneID int) error
	SelectPane(sessionID, windowID string, paneID int) error
	ResizePane(paneID int, direction string, amount int) error
	CapturePane(paneID int, history bool) (string, error)
	RespawnPane(paneID int, shell string) error

	// Key binding mutations.
	BindKey(table, key, cmd string) error
	UnbindKey(table, key string) error
	ListKeyBindings(table string) []KeyBinding

	// Option mutations.
	SetOption(scope, name, value string) error
	UnsetOption(scope, name string) error
	ListOptions(scope string) []OptionEntry

	// Server control.
	KillServer() error

	// UI / output.
	DisplayMessage(clientID, msg string) error
	SendKeys(paneID int, keys []string) error
	RunShell(cmd string, background bool) (string, error)

	// Buffer mutations.
	SetBuffer(name, data string) error
	DeleteBuffer(name string) error
	LoadBuffer(name, path string) error
	SaveBuffer(name, path string) error
	PasteBuffer(name string, paneID int) error
	ListBuffers() []BufferEntry

	// Layout mutations.
	// ApplyLayout applies a named preset ("even-horizontal", "even-vertical",
	// "main-horizontal", "main-vertical", "tiled"), the special names "next",
	// "prev", "even" (auto-select horizontal or vertical), or "undo" to
	// revert the last layout change, or a tmux-serialised layout string.
	ApplyLayout(sessionID, windowID, layoutSpec string) error
	// RotateWindow rotates pane positions in the window. forward=true shifts
	// pane 0 to position 1, etc.; the last pane wraps to position 0.
	RotateWindow(sessionID, windowID string, forward bool) error
	// ResizeWindow updates the window dimensions and re-applies the current
	// layout at the new size.
	ResizeWindow(sessionID, windowID string, cols, rows int) error

	// Window movement.
	MoveWindow(sessionID, windowID string, newIndex int) error
	SwapWindows(sessionID, aWindowID, bWindowID string) error
	FindWindow(sessionID, pattern string) (WindowView, error)

	// Window linking.
	// LinkWindow links the window srcWindowID from srcSessionID into
	// dstSessionID. index specifies the desired display index in the
	// destination session (-1 means append). afterIndex inserts at index+1,
	// beforeIndex inserts at index; both are ignored when index is -1.
	// selectWin selects the linked window in the destination session.
	// killExisting kills any window that currently holds index in the
	// destination session before inserting.
	LinkWindow(srcSessionID, srcWindowID, dstSessionID string, index int, afterIndex, beforeIndex, selectWin, killExisting bool) error
	// UnlinkWindow removes the window windowID from sessionID's window list
	// without killing the window itself. If kill is true and the window has
	// no remaining linked sessions after the removal, all panes are closed.
	UnlinkWindow(sessionID, windowID string, kill bool) error

	// Pane movement.
	SwapPane(sessionID, windowID string, paneA, paneB int) error
	BreakPane(sessionID, windowID string, paneID int) (WindowView, error)
	JoinPane(srcSessionID, srcWindowID string, srcPaneID int, dstSessionID, dstWindowID string) error

	// Environment mutations.
	SetEnvironment(scope, name, value string, remove bool) error
	ListEnvironment(scope string) []EnvironEntry

	// Server management.
	ShowMessages() []string
	LockServer() error
	LockClient(clientID string) error
	WaitFor(channel string) error
	SignalChannel(channel string)

	// Mode entry mutations.
	EnterCopyMode(clientID string, scrollback bool) error
	EnterChooseTree(clientID, sessionID, windowID string) error
	EnterCustomizeMode(clientID string) error
	EnterChooseBuffer(clientID, windowID string, items []ChooserItem, template string) error
	EnterChooseClient(clientID, windowID string, items []ChooserItem, template string) error
	EnterClockMode(clientID string, paneID int) error
	DisplayPopup(clientID, command, title string, cols, rows int) error
	DisplayMenu(clientID string, items []MenuEntry) error
	DisplayPanes(clientID string) error
	CommandPrompt(clientID, prompt, initialValue string) error
	ConfirmBefore(clientID, prompt, command string) error

	// Hook mutations.
	// SetHook registers a command to run when event fires.
	// Pass cmd="" to unregister all hooks for event.
	SetHook(event, cmd string) error
	// RunHook fires all registered hooks for event synchronously.
	RunHook(event string)

	// Client display mutations.
	// RefreshClient triggers a full redraw for the client identified by clientID.
	RefreshClient(clientID string) error
	// ResizeClient updates the dimensions of the client identified by clientID.
	ResizeClient(clientID string, cols, rows int) error
	// SuspendClient sends SIGTSTP to the client process identified by clientID.
	SuspendClient(clientID string) error

	// Server access control.
	// SetServerAccess adds or updates an ACL entry: allow/deny username with
	// optional write access. Pass allow=false to deny the user.
	SetServerAccess(username string, allow, write bool) error
	// DenyAllClients sets the server to refuse all new connections.
	DenyAllClients() error

	// Pane pipe mutations.
	// PipePane connects the output of a pane to a shell command.
	// shellCmd is the command to run; empty string stops an existing pipe.
	// inFlag routes the command's stdout back to the pane's stdin.
	// outFlag routes the pane's output to the command's stdin (default).
	// onceFlag only opens the pipe if the pane is not already piped.
	PipePane(paneID int, shellCmd string, inFlag, outFlag, onceFlag bool) error

	// MovePane moves a pane to a different window (analogous to JoinPane but
	// the source pane is detached from its current window before joining).
	MovePane(srcSessionID, srcWindowID string, srcPaneID int, dstSessionID, dstWindowID string) error

	// SlicePane creates a new pane whose initial content is taken from a
	// rectangular region of an existing pane's snapshot.
	SlicePane(sessionID, windowID string, paneID int) (PaneView, error)

	// RespawnWindow relaunches the shell or a given command in all dead panes
	// of the target window, optionally killing live panes first.
	RespawnWindow(sessionID, windowID, shell, dir string) error

	// ClearHistory discards the scrollback buffer of a pane.
	// If visibleToo is true the visible screen is also erased.
	ClearHistory(paneID int, visibleToo bool) error

	// ClearPane erases the visible content of a pane by injecting the ANSI
	// clear-screen sequence into its pseudo-terminal.
	ClearPane(paneID int) error
}

// ─── Argument types ───────────────────────────────────────────────────────────

// ArgSpec describes the flags and positional arguments a command accepts.
type ArgSpec struct {
	// Flags lists names of boolean flags (e.g. "d", "k").
	Flags []string
	// Options lists names of string-valued flags (e.g. "n", "s").
	// The "-t" option is added automatically when Target.Kind != TargetNone.
	Options []string
	// MinArgs is the minimum number of required positional arguments.
	MinArgs int
	// MaxArgs is the maximum number of allowed positional arguments.
	// -1 means unlimited.
	MaxArgs int
}

// ParsedArgs holds the result of parsing a command's arguments.
type ParsedArgs struct {
	// Flags holds the boolean flags that were present.
	Flags map[string]bool
	// Options holds string-valued flags and their values.
	Options map[string]string
	// Positional holds the non-flag arguments.
	Positional []string
}

// Flag returns true if the named flag was present (e.g. Flag("d") for -d).
func (p ParsedArgs) Flag(name string) bool { return p.Flags[name] }

// Option returns the value of a string-valued flag, or "" if absent.
func (p ParsedArgs) Option(name string) string { return p.Options[name] }

// ─── Target types ─────────────────────────────────────────────────────────────

// TargetKind describes what level of resolution a command's -t flag produces.
type TargetKind int

const (
	TargetNone    TargetKind = iota // command takes no target
	TargetSession                   // -t resolves to a session
	TargetWindow                    // -t resolves to a session:window
	TargetPane                      // -t resolves to a session:window.pane
)

// TargetSpec describes the -t flag semantics for a command.
type TargetSpec struct {
	// Kind is the resolution level required.
	Kind TargetKind
	// Optional means -t may be absent; the current client context is used.
	Optional bool
}

// Target is the resolved target after -t has been parsed and looked up.
type Target struct {
	Session SessionView
	Window  WindowView
	Pane    PaneView
	// Kind indicates how far resolution went.
	Kind TargetKind
}

// ─── Spec and Ctx ─────────────────────────────────────────────────────────────

// Ctx is the execution context passed to every command handler. All fields
// use value types or interfaces defined in this package, so handlers can be
// tested without a live server.
type Ctx struct {
	// Server provides read access to sessions and clients.
	Server Server
	// Mutator provides write access to server state. It may be nil when
	// the command is dispatched in a read-only context (e.g. some tests).
	Mutator Mutator
	// Client is the client that originated the command.
	// The zero ClientView indicates a non-client-originated command.
	Client ClientView
	// Target is the resolved -t target.
	Target Target
	// Args holds the parsed flags and positional arguments.
	Args ParsedArgs
	// Queue is the async command queue; handlers may enqueue follow-ups.
	Queue *Queue
}

// Spec describes a command that can be registered and dispatched.
type Spec struct {
	// Name is the canonical command name (e.g. "new-session").
	Name string
	// Alias lists alternative names (e.g. "new-s").
	Alias []string
	// Args describes expected flags and positional arguments.
	Args ArgSpec
	// Target describes what -t resolves to for this command.
	Target TargetSpec
	// Run is called when the command is dispatched.
	Run func(*Ctx) Result
}

// ─── Registry ─────────────────────────────────────────────────────────────────

// Registry holds a set of registered command Specs.
type Registry struct {
	specs map[string]*Spec
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{specs: make(map[string]*Spec)}
}

// Register adds spec to r. It returns an error if any name or alias is
// already registered.
func (r *Registry) Register(spec Spec) error {
	names := append([]string{spec.Name}, spec.Alias...)
	for _, name := range names {
		if _, dup := r.specs[name]; dup {
			return fmt.Errorf("command: duplicate registration for %q", name)
		}
	}
	ptr := &spec
	for _, name := range names {
		r.specs[name] = ptr
	}
	return nil
}

// Lookup returns the Spec registered under name, or nil if not found.
func (r *Registry) Lookup(name string) *Spec {
	return r.specs[name]
}

// List returns all registered Specs without duplicates (aliases share a Spec pointer).
func (r *Registry) List() []*Spec {
	seen := map[*Spec]bool{}
	var out []*Spec
	for _, s := range r.specs {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// Dispatch looks up name, parses rawArgs, resolves the target, and calls the
// handler. It uses store for target resolution and passes q as Ctx.Queue.
func (r *Registry) Dispatch(name string, rawArgs []string, store Server, client ClientView, q *Queue, mut ...Mutator) Result {
	spec := r.Lookup(name)
	if spec == nil {
		return Errorf("unknown command: %s", name)
	}

	var m Mutator
	if len(mut) > 0 {
		m = mut[0]
	}

	// Auto-inject -t into ArgSpec.Options when the command takes a target.
	argSpec := spec.Args
	if spec.Target.Kind != TargetNone {
		hasT := false
		for _, o := range argSpec.Options {
			if o == "t" {
				hasT = true
				break
			}
		}
		if !hasT {
			argSpec.Options = append(argSpec.Options, "t")
		}
	}

	parsed, err := parseArgs(argSpec, rawArgs)
	if err != nil {
		return Result{Err: err}
	}

	target, err := resolveTarget(spec.Target, parsed.Options["t"], store, client)
	if err != nil {
		return Result{Err: err}
	}

	ctx := &Ctx{
		Server:  store,
		Mutator: m,
		Client:  client,
		Target:  target,
		Args:    parsed,
		Queue:   q,
	}
	return spec.Run(ctx)
}

// ─── Default (global) registry ────────────────────────────────────────────────

// Default is the package-level Registry used by package-level Register/Lookup.
var Default = NewRegistry()

// Register adds spec to the Default registry. It panics if any name or alias
// is already registered. This is called from each builtin's init() function.
func Register(spec Spec) {
	if err := Default.Register(spec); err != nil {
		panic(err.Error())
	}
}

// Lookup returns the Spec registered under name in the Default registry, or
// nil if not found.
func Lookup(name string) *Spec { return Default.Lookup(name) }

// List returns all Specs in the Default registry without duplicates.
func List() []*Spec { return Default.List() }

// Dispatch is a package-level convenience that delegates to Default.Dispatch.
func Dispatch(name string, rawArgs []string, store Server, client ClientView, q *Queue, mut ...Mutator) Result {
	return Default.Dispatch(name, rawArgs, store, client, q, mut...)
}

// ─── Argument parsing ─────────────────────────────────────────────────────────

// parseArgs parses rawArgs according to spec, supporting:
//   - -f (boolean flag)
//   - -fg (combined flags)
//   - -o value (option with space)
//   - -ovalue (option concatenated)
//   - -- (end of flags)
func parseArgs(spec ArgSpec, rawArgs []string) (ParsedArgs, error) {
	flagSet := make(map[string]bool, len(spec.Flags))
	for _, f := range spec.Flags {
		flagSet[f] = true
	}
	optSet := make(map[string]bool, len(spec.Options))
	for _, o := range spec.Options {
		optSet[o] = true
	}

	flags := make(map[string]bool)
	opts := make(map[string]string)
	var positional []string

	i := 0
	for i < len(rawArgs) {
		arg := rawArgs[i]
		if arg == "--" {
			positional = append(positional, rawArgs[i+1:]...)
			break
		}
		if len(arg) < 2 || arg[0] != '-' {
			positional = append(positional, arg)
			i++
			continue
		}
		// arg is -xyz or similar
		chars := arg[1:]
		for j := 0; j < len(chars); j++ {
			name := string(chars[j])
			switch {
			case flagSet[name]:
				flags[name] = true
			case optSet[name]:
				if j+1 < len(chars) {
					// value is the rest of this arg: -tvalue
					opts[name] = chars[j+1:]
					j = len(chars) // consume rest
				} else {
					i++
					if i >= len(rawArgs) {
						return ParsedArgs{}, fmt.Errorf("flag -%s requires an argument", name)
					}
					opts[name] = rawArgs[i]
				}
			default:
				return ParsedArgs{}, fmt.Errorf("unknown flag: -%s", name)
			}
		}
		i++
	}

	if len(positional) < spec.MinArgs {
		return ParsedArgs{}, fmt.Errorf("requires at least %d argument(s), got %d", spec.MinArgs, len(positional))
	}
	if spec.MaxArgs >= 0 && len(positional) > spec.MaxArgs {
		return ParsedArgs{}, fmt.Errorf("accepts at most %d argument(s), got %d", spec.MaxArgs, len(positional))
	}

	return ParsedArgs{Flags: flags, Options: opts, Positional: positional}, nil
}
