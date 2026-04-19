// Package control implements control mode — tmux's -CC flag — a
// machine-readable event stream that replaces normal rendered output.
//
// # Boundary
//
// When a client connects with the Control flag set, the server writes
// control-mode lines to the client's output channel instead of rendered
// terminal bytes. The protocol is line-based text:
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
// The Writer in this package subscribes to session hooks on
// construction and produces these lines. Commands arriving from the
// client are parsed by package command as usual; their output is
// bracketed in %begin/%end blocks so the consumer can correlate
// request and response.
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
// Testable by driving a fake session.Server's event stream and
// asserting on the bytes the Writer produces. No real client needed.
//
// # Non-goals
//
// No support for tmux's older "control mode" variant without %begin
// framing. We target the modern iTerm2-integration shape.
package control
