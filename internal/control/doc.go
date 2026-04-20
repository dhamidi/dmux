// Package control implements control mode — tmux's -CC flag — a
// machine-readable event stream that replaces normal rendered output.
//
// # Protocol
//
// Control mode uses a line-based text protocol. Each line begins with a
// "%" sigil followed by a keyword and space-separated fields:
//
//	%output %<pane-id> <base64-bytes>
//	%session-changed <session-id> <name>
//	%window-add <window-id>
//	%window-close <window-id>
//	%window-pane-changed <window-id> <pane-id>
//	%layout-change <window-id> <layout-string>
//	%exit <reason>
//	%begin <unix-time> <seq-number> <flags>
//	%end   <unix-time> <seq-number> <flags>
//
// Commands arriving from the client are parsed by package command as
// usual; their output is bracketed in %begin/%end blocks so the
// consumer can correlate request and response by sequence number.
//
// # Event types
//
// Each protocol line corresponds to a concrete Go type that implements
// the [Event] interface:
//
//   - [OutputEvent] → %output
//   - [SessionChangedEvent] → %session-changed
//   - [WindowAddEvent] → %window-add
//   - [WindowCloseEvent] → %window-close
//   - [WindowPaneChangedEvent] → %window-pane-changed
//   - [LayoutChangeEvent] → %layout-change
//   - [ExitEvent] → %exit
//   - [BeginEvent] → %begin
//   - [EndEvent] → %end
//
// All event types are plain structs whose fields are strings or integers,
// making them safe to copy and free of references to live server objects.
//
// # EventSource interface
//
// The [EventSource] interface decouples the Writer from any concrete
// server or session type:
//
//	type EventSource interface {
//	    Subscribe(handler func(event Event)) (unsubscribe func())
//	}
//
// Subscribe registers a handler and returns a function that, when called,
// removes the handler. The production implementation is provided by the
// server package; tests supply a stub.
//
// # Writer
//
// [Writer] subscribes to an [EventSource] on construction and serialises
// each received [Event] as a protocol line to an injected [io.Writer].
// Output always goes to the supplied writer — never to os.Stdout. Call
// [Writer.Close] when the client disconnects to stop event delivery.
//
// # Why it's a separate package
//
// The output shape is entirely different from normal mode — no
// rendering, no cells, no escape sequences. Isolating it here means
// the server loop branches once at attach time and then each code
// path stays simple.
//
// # Testing
//
// The package is testable without a real server: supply a stub
// [EventSource] and an in-memory [bytes.Buffer] as the io.Writer, then
// push events through the stub and assert on the serialised bytes.
//
// # Non-goals
//
// No support for tmux's older "control mode" variant without %begin
// framing. We target the modern iTerm2-integration shape.
package control
