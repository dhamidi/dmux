// Package client is the thin gomux client: it connects to the server
// socket, identifies itself, forwards stdin and display output, and
// exits with the status the server tells it to.
//
// # Boundary
//
// Single entry point:
//
//	Run(cfg Config) int
//
// Returns the process exit code. Config carries the socket path,
// the argv to send as MSG_COMMAND, flags (control mode, readonly),
// and the open tty file handles.
//
// # What the client does
//
//  1. Dial the socket. If no server is listening, fork/exec one (Unix)
//     or start a detached server process (Windows) and retry.
//  2. Send VERSION and IDENTIFY_* messages: TERM, terminfo entries
//     or feature set, tty name, cwd, environment (filtered by the
//     server-side update-environment option on the other side),
//     client pid, flags. Finish with IDENTIFY_DONE.
//  3. Put its terminal in raw mode via package term. Install
//     signal / polling-based resize detection.
//  4. Enter the forwarding loop:
//     - read stdin → STDIN message
//     - read socket → STDOUT / STDERR → write to real terminal,
//       plus handle READ_* / WRITE_* file-RPC messages and EXIT
//     - resize detected → RESIZE message
//     - Ctrl-D on stdin while at the prompt? No — the detach key
//       is a server-side binding; the client just forwards bytes.
//  5. On EXIT, restore the terminal and return the given code.
//
// # What the client does NOT do
//
// Parse escape sequences. Render cells. Maintain any session state.
// Decode keys. It is a byte pump with an identify handshake and a
// tiny file-RPC responder.
//
// # In isolation
//
// Testable against a fake server (a goroutine writing canned
// proto.Messages into one end of a net.Pipe). The test asserts on
// what bytes reach the client's fake "terminal" file and what
// messages the client sent.
//
// # Non-goals
//
// The client is deliberately dumb to keep the protocol stable and
// cross-platform. Any feature that looks like it wants to live in
// the client (key tables, modes, layouts, etc.) belongs in the
// server instead.
package client
