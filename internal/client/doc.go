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
// Returns the process exit code. All I/O is explicit via Config:
//
//   - cfg.Conn (io.ReadWriter) — the already-dialled server connection.
//     The caller (typically cmd/dmux) is responsible for dialling
//     cfg.SocketPath and assigning the resulting net.Conn here before
//     calling Run. Run does not dial.
//   - cfg.In (io.Reader) — raw terminal input forwarded as STDIN messages.
//     Nil or ignored when cfg.ReadOnly is true.
//   - cfg.Out (io.Writer) — receives rendered bytes from STDOUT and STDERR
//     messages sent by the server.
//   - cfg.Size (term.SizeFunc) — queried once at startup to send the
//     initial RESIZE message; supply a stub returning fixed dimensions
//     in tests.
//   - cfg.RawMode (term.RawModeFunc) — called to put the terminal in raw
//     mode for the duration of the session; may be nil in tests.
//
// Run never reads os.Stdin or writes os.Stdout directly.
//
// # Protocol
//
// All server communication uses the [proto] framing layer (8-byte header +
// payload). No ad-hoc framing is used.
//
// # What the client does
//
//  1. Write VERSION to Conn.
//  2. Write IDENTIFY_FLAGS, IDENTIFY_TERM, IDENTIFY_CWD, IDENTIFY_ENVIRON,
//     IDENTIFY_CLIENTPID, IDENTIFY_FEATURES, IDENTIFY_DONE to Conn.
//  3. Write RESIZE with the dimensions returned by cfg.Size.
//  4. If cfg.Argv is non-empty, write COMMAND.
//  5. Enter raw mode via cfg.RawMode (if non-nil).
//  6. Start a goroutine pumping cfg.In → STDIN messages on Conn
//     (skipped when cfg.ReadOnly or cfg.In is nil).
//  7. Enter the server loop: read messages from Conn and dispatch:
//     - STDOUT / STDERR → write raw bytes to cfg.Out
//     - EXIT            → restore terminal, return the exit code
//     - READ_OPEN       → open the named file, stream READ messages, READ_DONE
//     - WRITE_OPEN      → send WRITE_READY, receive WRITE messages, write file
//     - connection EOF  → return 0 (clean server shutdown)
//
// # What the client does NOT do
//
// Parse escape sequences. Render cells. Maintain any session state.
// Decode keys. It is a byte pump with an identify handshake and a
// tiny file-RPC responder.
//
// # Testing in isolation
//
// Testable against a fake server using [net.Pipe]:
//
//	serverConn, clientConn := net.Pipe()
//	go fakeServer(serverConn) // reads handshake, writes STDOUT, sends EXIT
//	code := Run(Config{
//	    Conn:     clientConn,
//	    In:       bytes.NewReader(input),
//	    Out:      &bytes.Buffer{},
//	    Size:     func() (int, int, error) { return 24, 80, nil },
//	    ReadOnly: true,
//	})
//
// # Non-goals
//
// The client is deliberately dumb to keep the protocol stable and
// cross-platform. Any feature that looks like it wants to live in
// the client (key tables, modes, layouts, etc.) belongs in the
// server instead.
package client
