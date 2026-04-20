package control

// Event is the sealed interface for all control-mode events.
// Callers switch on the concrete type to extract fields.
// Every concrete Event type is a plain struct with only string or int fields,
// making it safe to copy and free of references to live session objects.
type Event interface {
	// eventKind returns a stable string label used internally for
	// logging and exhaustiveness checks. The sealed interface ensures
	// only types defined in this package can satisfy it.
	eventKind() string
}

// OutputEvent carries raw terminal bytes produced by a pane. The bytes are
// serialised as base64 in the %output line.
type OutputEvent struct {
	// PaneID is the pane identifier (e.g. "0", "1"). The %output line
	// prefixes it with "%" to produce the canonical tmux pane token.
	PaneID string
	// Data is the raw output bytes from the pane.
	Data []byte
}

func (OutputEvent) eventKind() string { return "output" }

// SessionChangedEvent signals that the client's active session has changed.
// It maps to the %session-changed protocol line.
type SessionChangedEvent struct {
	// SessionID is the stable session identifier.
	SessionID string
	// Name is the human-readable session name.
	Name string
}

func (SessionChangedEvent) eventKind() string { return "session-changed" }

// WindowAddEvent signals that a new window was added to a session.
// It maps to the %window-add protocol line.
type WindowAddEvent struct {
	// WindowID is the window identifier.
	WindowID string
}

func (WindowAddEvent) eventKind() string { return "window-add" }

// WindowCloseEvent signals that a window has been closed.
// It maps to the %window-close protocol line.
type WindowCloseEvent struct {
	// WindowID is the window identifier.
	WindowID string
}

func (WindowCloseEvent) eventKind() string { return "window-close" }

// WindowPaneChangedEvent signals that the active pane within a window changed.
// It maps to the %window-pane-changed protocol line.
type WindowPaneChangedEvent struct {
	// WindowID is the window identifier.
	WindowID string
	// PaneID is the new active pane identifier.
	PaneID string
}

func (WindowPaneChangedEvent) eventKind() string { return "window-pane-changed" }

// LayoutChangeEvent signals that the pane layout within a window changed.
// It maps to the %layout-change protocol line.
type LayoutChangeEvent struct {
	// WindowID is the window identifier.
	WindowID string
	// Layout is the serialised layout string (the same format tmux uses in
	// select-layout and layout-change events).
	Layout string
}

func (LayoutChangeEvent) eventKind() string { return "layout-change" }

// ExitEvent signals that the server or session is shutting down.
// It maps to the %exit protocol line.
type ExitEvent struct {
	// Reason is a short human-readable description of why the exit occurred,
	// or an empty string when the reason is not specified.
	Reason string
}

func (ExitEvent) eventKind() string { return "exit" }

// BeginEvent marks the start of a bracketed command-output block.
// Every BeginEvent is paired with an EndEvent carrying the same Number.
// It maps to the %begin protocol line.
type BeginEvent struct {
	// Time is the Unix timestamp (seconds) when the command started.
	Time int64
	// Number is the command sequence number used to correlate request/response.
	Number int
	// Flags is a bitmask of protocol flags (typically 0 for normal output).
	Flags int
}

func (BeginEvent) eventKind() string { return "begin" }

// EndEvent marks the end of a bracketed command-output block.
// It maps to the %end protocol line.
type EndEvent struct {
	// Time is the Unix timestamp (seconds) when the command completed.
	Time int64
	// Number is the command sequence number (must match the paired BeginEvent).
	Number int
	// Flags is a bitmask of protocol flags.
	Flags int
}

func (EndEvent) eventKind() string { return "end" }

// SessionRenamedEvent signals that a session was renamed.
// It maps to the %session-renamed protocol line.
type SessionRenamedEvent struct {
	SessionID string
	Name      string
}

func (SessionRenamedEvent) eventKind() string { return "session-renamed" }

// SessionsChangedEvent signals that the set of sessions changed (created or destroyed).
// It maps to the %sessions-changed protocol line.
type SessionsChangedEvent struct{}

func (SessionsChangedEvent) eventKind() string { return "sessions-changed" }

// WindowRenamedEvent signals that a window was renamed.
// It maps to the %window-renamed protocol line.
type WindowRenamedEvent struct {
	WindowID string
	Name     string
}

func (WindowRenamedEvent) eventKind() string { return "window-renamed" }

// PaneModeChangedEvent signals that a pane entered or exited a mode.
// It maps to the %pane-mode-changed protocol line.
type PaneModeChangedEvent struct {
	PaneID string
}

func (PaneModeChangedEvent) eventKind() string { return "pane-mode-changed" }

// ClientDetachedEvent signals that a client detached.
// It maps to the %client-detached protocol line.
type ClientDetachedEvent struct {
	Name string
}

func (ClientDetachedEvent) eventKind() string { return "client-detached" }

// UnlinkedWindowAddEvent signals that a window was added outside the current session.
// It maps to the %unlinked-window-add protocol line.
type UnlinkedWindowAddEvent struct {
	WindowID string
}

func (UnlinkedWindowAddEvent) eventKind() string { return "unlinked-window-add" }

// ContinueEvent signals that pane output has resumed.
// It maps to the %continue protocol line.
type ContinueEvent struct {
	PaneID string
}

func (ContinueEvent) eventKind() string { return "continue" }

// PauseEvent signals that pane output has been paused (backpressure).
// It maps to the %pause protocol line.
type PauseEvent struct {
	PaneID string
}

func (PauseEvent) eventKind() string { return "pause" }

// ErrorEvent marks the end of a failed command-output block.
// It maps to the %error protocol line.
type ErrorEvent struct {
	Time   int64
	Number int
	Flags  int
}

func (ErrorEvent) eventKind() string { return "error" }
