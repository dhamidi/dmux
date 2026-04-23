// Command dmux is a terminal multiplexer modeled closely on tmux.
//
// # Invocation
//
// Running dmux with no arguments:
//
//  1. Resolves the socket path (see internal/sockpath).
//  2. Attempts to connect to a running server on that socket.
//  3. If no server is running, forks and starts one (see internal/server).
//  4. Sends a new-session command followed by attach-session via
//     the wire protocol (see internal/proto).
//  5. Runs the client loop (see internal/client) until the session
//     detaches or the server exits.
//
// dmux aims for byte-for-byte behavioral compatibility with tmux where
// practical. It targets three terminal emulators:
//
//   - xterm.js (VS Code and embedded)
//   - Ghostty (Linux and macOS)
//   - Windows Terminal (recent versions)
//
// Other clients may work but are not a support target.
//
// # Platforms
//
// dmux supports Linux, macOS, and Windows. Platform-specific code is
// confined to internal/platform, internal/pty, and internal/socket.
//
// This file contains only flag parsing and dispatch to either the client
// or a standalone server bootstrap. No session, pane, or terminal logic
// lives here.
package main
