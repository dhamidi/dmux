// Package control implements control mode — tmux's -CC flag — a
// machine-readable event stream that replaces normal rendered output.
//
// # Boundary
//
//	func NewWriter(out io.Writer, src EventSource) *Writer
//	(*Writer).Close() error
//
//	type EventSource interface {
//	    Subscribe(handler func(Event)) (unsubscribe func())
//	}
//
//	type Event interface { isControlEvent() }
//	// concrete events: PaneOutput, SessionChanged, WindowAdd,
//	// WindowClose, WindowPaneChanged, LayoutChange, Exit
//
// Writer takes any io.Writer (the client connection in production, a
// bytes.Buffer in tests) and any EventSource (the running server in
// production, a test harness that fires synthetic events). It does not
// import session.
//
// The protocol is line-based text:
//
//	%output %<pane> <base64-bytes>
//	%session-changed <session-id> <name>
//	%window-add <window-id>
//	%window-close <window-id>
//	%window-pane-changed <window-id> <pane-id>
//	%layout-change <window-id> <layout-string>
//	%exit <reason>
//	%begin <time> <number> <flags>    ... output ... %end <...>
//
// Commands arriving from the client are parsed by package command as
// usual; their output is bracketed in %begin/%end blocks so the
// consumer can correlate request and response.
//
// # I/O surfaces
//
//   - Writes bytes to the caller-supplied io.Writer.
//   - Reads time.Now() for %begin timestamps (clock injectable for
//     deterministic tests).
//
// No filesystem, no network, no goroutines beyond the EventSource
// callback's own.
//
// # Why it's a separate package
//
// The output shape is entirely different from normal mode — no
// rendering, no cells, no escape sequences. Isolating it here means
// the server loop branches once at attach time and then each code
// path stays simple.
//
// # In isolation
//
// Testable by constructing a Writer with a bytes.Buffer and a stub
// EventSource, firing events, and asserting on the buffer. No
// session, no client, no socket.
//
// # Non-goals
//
// No support for tmux's older "control mode" variant without %begin
// framing. We target the modern iTerm2-integration shape.
package control
