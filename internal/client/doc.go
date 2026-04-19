// Package client is the thin dmux client: it connects to the server
// socket, identifies itself, forwards stdin and display output, and
// exits with the status the server tells it to.
//
// # Boundary
//
// Single entry point:
//
//	Run(cfg Config) int
//
//	type Config struct {
//	    Dialer    Dialer       // returns a net.Conn to the server
//	    Starter   ServerStarter // called if Dialer fails (no server yet)
//	    TTYIn     *os.File     // raw stdin
//	    TTYOut    *os.File     // raw stdout
//	    TermName  string       // $TERM, read by the caller
//	    Caps      term.CapSource
//	    Env       []string     // captured environ; passed to IDENTIFY
//	    Cwd       string       // captured cwd; passed to IDENTIFY
//	    Argv      []string     // sent as COMMAND
//	    Flags     ClientFlags  // control mode, readonly, etc.
//	}
//
//	type Dialer interface {
//	    Dial() (net.Conn, error)
//	}
//
//	type ServerStarter interface {
//	    Start() error    // fork/exec the server, then return
//	}
//
// Returns the process exit code. Every external dependency — the socket
// path, how to start a server, how to read $TERM and the environ —
// is injected. cmd/dmux is the only place that turns command-line flags
// into a real Dialer (net.Dial("unix", path)) and a real ServerStarter.
//
// # What the client does
//
//  1. Call Dialer.Dial(). If it fails, call ServerStarter.Start() and
//     retry the dial.
//  2. Send VERSION and IDENTIFY_* messages: TermName, terminfo entries
//     or feature set, tty name, Cwd, Env (already captured by the
//     caller and possibly filtered), client pid, Flags. Finish with
//     IDENTIFY_DONE.
//  3. Put TTYIn/TTYOut into raw mode via package term. Install
//     signal / polling-based resize detection on the tty handles.
//  4. Enter the forwarding loop:
//     - read stdin → STDIN message
//     - read socket → STDOUT / STDERR → write to real terminal,
//       plus handle READ_* / WRITE_* file-RPC messages and EXIT
//     - resize detected → RESIZE message
//     - Ctrl-D on stdin while at the prompt? No — the detach key
//       is a server-side binding; the client just forwards bytes.
//  5. On EXIT, restore the terminal and return the given code.
//
// # I/O surfaces
//
//   - Dials a socket via the injected Dialer.
//   - Optionally starts a server via the injected ServerStarter.
//   - Reads/writes TTYIn and TTYOut.
//   - Reads/writes the net.Conn returned by Dialer.
//   - File-RPC: opens files named by the server's READ_OPEN /
//     WRITE_OPEN messages, scoped to whatever the client process can
//     access.
//
// No environment reads, no /etc/passwd, no signal handler installation
// of its own — the caller hands those in.
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
