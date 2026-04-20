// Package server is the long-lived dmux server process: the event
// loop, the client accept path, timers, redraw scheduling, and the
// glue that ties every lower-tier package together.
//
// # Boundary
//
// Single entry point:
//
//	Run(cfg Config) error
//
// Run blocks until a shutdown signal is delivered on cfg.Signals or a
// connected client sends MsgShutdown. It returns nil on clean shutdown.
//
// # Config struct
//
// Config expresses every I/O dependency explicitly. Run never calls
// os.Stderr, os.Getenv, time.Now, or signal.Notify directly.
//
//	type Config struct {
//	    // Listener accepts incoming client connections.
//	    // The caller opens the socket before constructing Config.
//	    // Tests may use a net.Pipe-backed listener.
//	    Listener net.Listener
//
//	    // Log is the destination for server diagnostic output.
//	    // Defaults to io.Discard if nil.
//	    Log io.Writer
//
//	    // Signals receives OS signals. Any received value is treated as a
//	    // graceful-shutdown trigger (SIGTERM, SIGHUP). Wire via
//	    // os/signal.Notify in cmd/; send synthetic values in tests.
//	    Signals <-chan os.Signal
//
//	    // Now returns the current time for timer and debounce logic.
//	    // Defaults to time.Now if nil; inject a fixed clock in tests.
//	    Now Clock
//
//	    // OnDirty, when non-nil, is called whenever a client is marked
//	    // dirty for redraw. Tests use this hook to observe scheduling
//	    // without a full rendering layer.
//	    OnDirty func(id session.ClientID)
//	}
//
// The Clock type alias (func() time.Time) is also exported so callers
// can name the type when constructing test stubs.
//
// # What the loop owns
//
//   - A net.Listener and an accepted-client goroutine pool
//   - A *session.Server (the state)
//   - A *command.Queue (work to do)
//   - A timer wheel for status ticks, display-panes timeouts,
//     auto-rename polling, silence/activity alerts, job timeouts
//   - A redraw debouncer that coalesces per-client redraw requests
//   - Signal handling (SIGHUP/SIGTERM → graceful shutdown;
//     SIGWINCH is ignored — clients report their own size via proto)
//
// # Per-client goroutine
//
// Each attached client gets one goroutine that:
//
//  1. Reads proto messages (STDIN, RESIZE, COMMAND, DETACH, ...)
//  2. Translates STDIN bytes through keys.Decoder, dispatches
//     through the client's current KeyTable, enqueues commands
//  3. Writes rendered frames (from render.Compose → term.Flush
//     encoding into bytes for the wire) or control-mode events
//  4. Handles the file-RPC messages (READ_*, WRITE_*) initiated
//     by server-side commands like `load-buffer -`
//
// # Synchronized panes
//
// When the "synchronize-panes" window option is on, every STDIN byte
// sequence that is not consumed by a key binding is forwarded to all
// panes in the active window, not just the active pane. The rendered
// frame for a synchronized window also shows a visual indicator ('*'
// markers at the right and bottom edges of each pane).
//
// # Redraw cadence
//
// Never faster than the configured refresh interval. Per-client
// dirty tracking: a client only redraws when its session, window,
// layout, any visible pane's render state, status line, or overlays
// change.
//
// # In isolation
//
// Bootable against an in-process listener, connected to from a test
// with a raw net.Conn sending synthesized proto.Message values. Full
// end-to-end behavior can be asserted without any real TTY — the
// "rendered output" bytes come back on the socket and can be parsed
// by a test.
//
// # Non-goals
//
// No TTY handling of its own. No direct pane ownership — panes live
// inside windows inside sessions inside session.Server. The server
// package is orchestration only.
package server
