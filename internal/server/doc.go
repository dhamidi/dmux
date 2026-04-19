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
// Config carries the socket path, initial .dmux.conf path, and
// platform hooks (daemonize on Unix, Windows service integration if
// ever added). Run blocks until the server exits.
//
// # What the loop owns
//
//   - A UNIX-domain listener and an accepted-client goroutine pool
//   - A *session.Server (the state)
//   - A *command.Queue (work to do)
//   - A timer wheel for status ticks, display-panes timeouts,
//     auto-rename polling, silence/activity alerts, job timeouts
//   - A redraw debouncer that coalesces per-client redraw requests
//   - Signal handling (SIGHUP/SIGTERM on Unix → graceful shutdown;
//     SIGWINCH is ignored here — the server's own terminal isn't
//     the interesting one, clients report their size via proto)
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
// # Redraw cadence
//
// Never faster than the configured refresh interval. Per-client
// dirty tracking: a client only redraws when its session, window,
// layout, any visible pane's render state, status line, or overlays
// change.
//
// # In isolation
//
// Bootable against a tempdir socket, connected to from a test with a
// raw net.Conn sending synthesized proto.Message values. Full
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
