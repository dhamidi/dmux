// Package proto defines the wire protocol between the gomux client and
// server.
//
// # Boundary
//
// Pure encode/decode. Reads and writes framed, typed messages over an
// io.Reader / io.Writer. Does no I/O loop management, owns no goroutines,
// and knows nothing about sessions, panes, or terminals. A test can
// round-trip every message type through net.Pipe without booting a server.
//
// # Wire format
//
// Each message is a fixed 8-byte header (4-byte big-endian type, 4-byte
// big-endian payload length) followed by payload bytes. Payloads are
// type-specific and use a minimal self-describing encoding — no external
// serialization library.
//
// # Message categories
//
// Identify:   IDENTIFY_FLAGS, IDENTIFY_TERM, IDENTIFY_TERMINFO,
//             IDENTIFY_TTYNAME, IDENTIFY_CWD, IDENTIFY_ENVIRON,
//             IDENTIFY_CLIENTPID, IDENTIFY_FEATURES, IDENTIFY_DONE.
//             Sent by the client on attach so the server knows what
//             terminal capabilities to emit for and where to spawn
//             new panes.
//
// Session:    COMMAND, RESIZE, DETACH, EXIT, EXITED, SHUTDOWN, VERSION.
//             The normal attached-client lifecycle.
//
// Data:       STDIN, STDOUT, STDERR. Raw bytes flowing between the
//             client's real terminal and the server's rendered output.
//
// File RPC:   READ_OPEN, READ, READ_DONE, WRITE_OPEN, WRITE, WRITE_READY,
//             WRITE_CLOSE. Lets the server ask the client to read from
//             or write to a file the client has access to. Supports
//             `load-buffer -`, `save-buffer -`, `display-message -p`,
//             and similar features without needing fd-passing.
//
// # Versioning
//
// The first message on any connection is VERSION. Incompatible versions
// are rejected with EXIT before any other exchange.
//
// # Non-goals
//
// Not a streaming RPC framework. Not an IDL. Not pluggable. Message
// types are a closed enum defined in this package.
package proto
