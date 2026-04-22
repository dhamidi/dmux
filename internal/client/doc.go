// Package client implements the dmux client process.
//
// The client is a thin byte pump between the user's real terminal and
// the dmux server. It holds no session, window, pane, or key-binding
// state; all of that lives on the server.
//
// # Responsibilities
//
//  1. Resolve the socket path (internal/sockpath) and connect or start
//     the server (internal/socket.DialOrStart).
//  2. Put the real terminal into raw mode (internal/tty).
//  3. Probe the terminal's capability profile (internal/termcaps).
//  4. Send an Identify frame with the profile, environment, initial
//     cell size, cwd, and tty name.
//  5. Run three concurrent goroutines, all derived from a per-client
//     `context.Context`:
//     - Read bytes from stdin; forward as Input frames.
//     - Read frames from the server; act on them
//     (Output -> stdout, Exit, CommandResult).
//     - Watch for resize events (SIGWINCH on Unix, console events on
//     Windows); send Resize frames.
//  6. On exit, cancel the context, wait for goroutines to drain,
//     restore terminal state, print any exit message.
//
// # Dumb client
//
// The client does not parse terminal input into events. Raw stdin
// bytes are streamed to the server, where internal/termin does the
// parsing in a per-client state machine. The client also does not
// interpret server output beyond writing it to stdout.
//
// The one exception is capability probing at startup: the client
// sends a DA2 query and an optional KKP-detect query, collects
// responses for a short timeout, and includes the results in the
// Identify frame. After that, it is pure I/O.
//
// # Corresponding tmux code
//
// client.c, minus the fd-passing, pledge calls, and the split between
// "wait" and "attached" dispatch states (bytestream transport flattens
// these).
package client
