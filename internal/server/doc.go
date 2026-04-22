// Package server is the long-running dmux process that owns sessions,
// windows, and attached clients, and orchestrates panes.
//
// # Concurrency model
//
// The server's main goroutine owns:
//
//   - the session/window/winlink registry
//   - per-client state (current session, current window, key tables,
//     command queue, termcaps profile, termin parser, termout
//     renderer, per-pane frame cache)
//   - the global cmdq
//
// It does NOT own:
//
//   - vt.Terminal instances. Each Pane owns its own (see internal/pane).
//   - pty fds. Each Pane owns its own.
//   - socket I/O. Each Client has reader and writer goroutines.
//
// The main goroutine is the single-writer for everything in its
// list above. Edge goroutines own their I/O and translate to typed
// Events on a single events channel:
//
//	type Event interface { eventTag() }
//
//	type ClientFrame          struct { Client *Client; Frame proto.Frame }
//	type ClientGone           struct { Client *Client; Err error }
//	type PaneSnapshot         struct { Pane *pane.Pane; Grid vt.Grid }
//	type PaneExited           struct { Pane *pane.Pane; Status pty.ExitStatus }
//	type ContinuationReady    struct { ItemID cmdq.ItemID; Value any }
//	type AcceptedClient       struct { Conn net.Conn }
//	type IdentifyCompleted    struct { Client *Client }
//	type MessageTimerFired    struct { Client *Client }
//	type PaneBell             struct { Pane *pane.Pane }
//	type ShutdownRequested    struct { Reason ShutdownReason }
//
// All Event types live in the `events.go` file in this package.
// The full set is declared in one place so additions are visible at
// review time. Every event has exactly one producer and exactly one
// handler on the main loop.
//
//	for {
//	    select {
//	    case ev := <-events:    handle(ev)
//	    case sig := <-signals:  handleSignal(sig)
//	    case <-tick.C:          handleTick()
//	    case <-rootCtx.Done():  return
//	    }
//	}
//
// # Identify ordering and CommandList buffering
//
// Per proto: a client must send Identify before any other frame.
// But in the common bootstrap path (`dmux` with no args), the client
// immediately sends a CommandList containing new-session +
// attach-session right after Identify — the two frames can arrive
// in the server's socket buffer before the Identify is processed.
//
// The per-client reader goroutine parses frames and pushes them as
// ClientFrame events. The server main goroutine's ClientFrame
// handler special-cases the pre-Identify state:
//
//   - Identify arrives: configure the client, emit IdentifyCompleted,
//     drain the pre-Identify CommandList buffer in arrival order.
//   - CommandList arrives before Identify: push onto a fixed-size
//     per-client buffer (cap 10). If full, reply Exit{ProtocolError}
//     and cancel the client's context.
//   - Input / Resize / CapsUpdate / Bye arrive before Identify:
//     Exit{ProtocolError}. These only make sense on a configured client.
//
// 10 CommandLists is an order-of-magnitude safety margin; the
// bootstrap path sends exactly one.
//
// `handle` is plain mutation. It walks the registry, drains command
// queues, asks panes for snapshots when needed, calls
// termout.Render, pushes Output bytes to per-client writer channels.
//
// # Why this is not single-threaded but is single-state-owner
//
// Per-pane goroutines do real work in parallel: vt.Feed for pane A
// runs concurrently with vt.Feed for pane B. The single-state-owner
// property only applies to *server state*: sessions, windows,
// clients, command queues, key tables. That state has exactly one
// writer (the main goroutine), so no locks.
//
// Per-pane state (vt.Terminal, viewport) has a different
// single-writer: the pane's own goroutine. Locks aren't needed there
// either, because the pane goroutine is sole writer to its own
// terminal.
//
// The discipline that makes this work: edge goroutines and pane
// goroutines never reach into the server's state graph. They emit
// events. The main goroutine never reaches into a pane's vt
// directly; it asks via the pane's controlCh.
//
// # Cancellation
//
// Every long-lived goroutine takes a `context.Context`. The server
// holds a root context. Each Client has a child context cancelled
// when the client disconnects. Each Pane has a child context that
// can be the session's child or independent depending on how the
// pane is attached. Pty readers and socket readers take child
// contexts of their owners.
//
// Cancellation cascades naturally. No CLIENT_DEAD flag, no manual
// destruction ordering, no reference counts. See `docs/go-patterns.md`.
//
// # Shutdown
//
// kill-server, the last session closing with exit-empty set, or
// SIGTERM all cancel the root context. The shutdown path is
// deliberately unceremonious:
//
//  1. rootCtx cancel cascades to every child context.
//  2. Per-client writer goroutines observe ctx.Done and close their
//     sockets; pending Output frames are dropped.
//  3. Pane reader goroutines' pty fds are closed, unblocking Read
//     with an error; pane goroutines exit without waiting for child
//     process reap.
//  4. pty.Close on Unix sends SIGHUP to each pane's process group
//     and returns immediately; Windows uses
//     ConptyClosePseudoConsoleTimeout(h, 0). We do NOT wait for
//     children to exit. The server process exits; children become
//     orphans and are reaped by init/session leader.
//  5. Socket listener closes; pending accepts fail immediately.
//  6. server.Run returns; the server process exits.
//
// No graceful timeout, no drain window. Force close. The rationale:
// dmux holds no persistent state across restart — the user's panes'
// shell state is the only thing that survives detach/attach, and
// that state is in the child processes, which keep running if the
// user detaches rather than kills the server. When the user asks
// for kill-server, they're explicitly asking to lose that state.
//
// Signal handlers translate SIGTERM/SIGINT to a ShutdownRequested
// event so the main loop can log the reason before cancelling.
// SIGKILL is not handled (unhandlable).
//
// # Startup
//
//  1. Initialize root context and events channel.
//  2. Install signal handlers (internal/platform).
//  3. Initialize the wasm VT runtime (internal/vt) once, shared by
//     all panes.
//  4. Build option scopes (internal/options): server, then session
//     defaults, then window defaults — populated from the closed
//     options table.
//  5. Parse and register default key bindings (M2; in M1 the key
//     table is empty, all keys pass through to the focused pane).
//  6. Open the socket listener (internal/socket); spawn the accept
//     goroutine.
//  7. If launched with a bootstrap command (e.g. new-session from
//     the first attaching client), enqueue it on the global cmdq.
//  8. Enter the main loop.
//
// # Per-event handling
//
// `AcceptedClient` constructs a Client struct, spawns its reader
// and writer goroutines, registers it.
//
// `ClientFrame` dispatches by frame type:
//
//   - Identify: configure the client's termcaps.Profile, construct
//     its termin.Parser and termout.Renderer; for every pane in the
//     session this client will see, push attached-profiles update.
//   - Input: feed bytes to client.parser; for each event match
//     against client.keyTable; on hit, append a List of cmdq.Items;
//     on miss, route to the focused pane via Pane.SendKey.
//   - Resize: update client.size, recompute pane sizes, send Resize
//     on every affected pane's controlCh.
//   - Command: parse via cmd.ParseArgv, append items.
//   - Exit: cancel client.ctx; pane goroutines learn via the next
//     attached-profiles update.
//
// `PaneSnapshot` updates the per-client frame cache for any client
// whose focused pane is this pane, then renders.
//
// `PaneExited` runs the pane's death routine: removes the pane from
// its window; if the window has no remaining panes, removes the
// window from its session; if the session has no windows, destroys
// the session and detaches every attached client.
//
// `ContinuationReady` looks up the parked cmdq item and runs its
// continuation. Result handling is identical to a fresh Exec.
//
// # Render path
//
// Render is push-driven by panes plus pull-driven by client events:
//
//   - When a pane has new content, its goroutine produces a snapshot
//     (rate-limited per pane) and pushes it as a `PaneSnapshot`
//     event. The main loop updates per-client frame caches.
//   - When a client's view needs refresh for other reasons (resize,
//     window switch, status update), the main loop asks the focused
//     pane for an immediate snapshot via Pane.Snapshot(reply).
//
// In both cases the main loop calls termout.Render against the
// snapshot plus the cached previous frame plus the client's profile,
// then pushes Output bytes to the client's writer channel. The
// writer channel is buffered; a slow client backpressures naturally
// and may drop intermediate frames if the renderer's frame cache
// detects coalescing is safe.
//
// Status line composition (internal/status) happens here too: each
// frame Output is the pane content plus the status line for that
// client.
//
// # Attached-client representation
//
// A server-side Client struct holds:
//
//   - net.Conn plus the writer channel and writer goroutine
//   - context.Context for cancellation
//   - termcaps.Profile
//   - termin.Parser (stateful)
//   - termout.Renderer (stateful, per-pane frame cache)
//   - current session, current window, active key table
//   - cmdq.List
//
// # Interface
//
//	Run(cfg Config) error  // blocks until last client leaves and sessions empty
//
// External callers (cmd/dmux) only see Run.
//
// # Corresponding tmux code
//
// tmux's server.c plus server-client.c plus proc.c. The dispatch
// switch in server-client.c:server_client_dispatch maps to the
// ClientFrame handler here. tmux's libevent setup in proc.c is
// replaced by the goroutine + select model. tmux's single-threaded
// constraint forced everything onto one event loop; per-pane
// goroutines mean a busy pane no longer blocks unrelated work.
package server
