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
//	type Config struct {
//	    Listener   net.Listener      // pre-bound; caller chose path/perms
//	    ConfigSrc  io.Reader         // initial .dmux.conf contents; nil ok
//	    Spawner    job.Spawner       // for run-shell, if-shell, #(...)
//	    PTYStarter pty.Starter       // wraps pty.Start; injectable for tests
//	    OS         osinfo.OS         // for automatic-rename
//	    Shell      shell.Sources     // env / passwd lookup for default-shell
//	    Clock      func() time.Time  // status ticks, timeouts
//	    Signals    SignalSource      // SIGHUP/SIGTERM channel; stub in tests
//	}
//
// The server takes every external dependency as an interface or pre-built
// value. It opens no sockets, reads no env, installs no signal handlers
// of its own — the caller does, and hands them in. cmd/dmux is the only
// place that does the actual binding to net.Listen, os/signal,
// os.Environ, etc.
//
// Run blocks until the server exits.
//
// # What the loop owns
//
//   - The injected Listener and an accepted-client goroutine pool
//   - A *session.Server (the state)
//   - A *command.Queue (work to do)
//   - A timer wheel for status ticks, display-panes timeouts,
//     auto-rename polling, silence/activity alerts, job timeouts
//     (driven by the injected Clock)
//   - A redraw debouncer that coalesces per-client redraw requests
//   - The injected SignalSource for graceful shutdown
//
// # I/O surfaces
//
//   - Accepts connections from Listener.Accept().
//   - Reads/writes proto.Messages on each accepted conn.
//   - Reads ConfigSrc once at startup.
//   - Spawns goroutines: one per accepted client, plus the per-pane
//     PTY copy goroutines owned by package pane.
//
// Everything else (binding the socket, installing real signal handlers,
// reading /etc/dmux.conf, daemonizing on Unix) happens in cmd/dmux
// before Run is called.
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
