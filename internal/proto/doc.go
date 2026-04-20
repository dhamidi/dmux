// Package proto defines the wire protocol between the dmux client and
// server.
//
// # Boundary
//
// Pure encode/decode. Reads and writes framed, typed messages over an
// [io.Reader] / [io.Writer]. Does no I/O loop management, owns no goroutines,
// and knows nothing about sessions, panes, or terminals. A test can
// round-trip every message type through a [bytes.Buffer] without booting
// a server or opening a network connection.
//
// The package has zero imports of other internal/* packages. All message
// type fields are primitive (bool, int, uint16, uint32, int32, string,
// []byte, []string).
//
// # Wire format
//
// Each message is a fixed 8-byte header followed by a variable-length
// payload:
//
//	+---+---+---+---+---+---+---+---+
//	|   MsgType (uint32 BE)  |   payload length (uint32 BE)  |
//	+---+---+---+---+---+---+---+---+
//	|            payload bytes …                              |
//
// MsgType is a big-endian uint32. Payload length is a big-endian uint32.
// The maximum payload size is 16 MiB. Messages with no body carry a
// zero-length payload and no payload bytes follow the header.
//
// Payload field encodings:
//   - string:   4-byte big-endian length followed by UTF-8 bytes.
//   - []string: 4-byte big-endian count, then each string as above.
//   - uint16:   2 bytes big-endian.
//   - uint32:   4 bytes big-endian.
//   - int32:    4 bytes big-endian (two's-complement cast to/from uint32).
//   - []byte:   raw bytes occupying the entire payload.
//   - empty:    zero-length payload (no payload bytes in the frame).
//
// # I/O contract
//
// [ReadMsg] and [WriteMsg] are the only entry points for the framing layer.
// Both accept plain [io.Reader] / [io.Writer] interfaces. No [net.Conn] or
// other concrete type is required. An in-memory [bytes.Buffer] is a valid
// transport for tests.
//
// # Message categories
//
// ## Identify
//
// Sent by the client on attach so the server knows what terminal
// capabilities to emit for and where to spawn new panes.
//
//   - [MsgIdentifyFlags]     ([IdentifyFlagsMsg])    — client flags bitmask.
//   - [MsgIdentifyTerm]      ([IdentifyTermMsg])     — TERM variable value.
//   - [MsgIdentifyTerminfo]  ([IdentifyTerminfoMsg]) — raw terminfo data bytes.
//   - [MsgIdentifyTTYName]   ([IdentifyTTYNameMsg])  — path to client's controlling TTY.
//   - [MsgIdentifyCWD]       ([IdentifyCWDMsg])      — client's working directory.
//   - [MsgIdentifyEnviron]   ([IdentifyEnvironMsg])  — environment "KEY=VALUE" pairs.
//   - [MsgIdentifyClientPID] ([IdentifyClientPIDMsg])— client process ID.
//   - [MsgIdentifyFeatures]  ([IdentifyFeaturesMsg]) — client feature-flag bitmask.
//   - [MsgIdentifyDone]      ([IdentifyDoneMsg])     — end of identify sequence.
//
// ## Session
//
// The normal attached-client lifecycle.
//
//   - [MsgVersion]  ([VersionMsg])  — protocol version; first message on every connection.
//   - [MsgCommand]  ([CommandMsg])  — command argv from client to server.
//   - [MsgResize]   ([ResizeMsg])   — terminal resize (width × height in cells).
//   - [MsgDetach]   ([DetachMsg])   — client requests detach.
//   - [MsgExit]     ([ExitMsg])     — server tells client to exit with a status code.
//   - [MsgExited]   ([ExitedMsg])   — server reports that a child process has exited.
//   - [MsgShutdown] ([ShutdownMsg]) — request or confirm server shutdown.
//
// ## Data
//
// Raw bytes flowing between the client's real terminal and the server's
// rendered output.
//
//   - [MsgStdin]  ([StdinMsg])  — input bytes from the client terminal.
//   - [MsgStdout] ([StdoutMsg]) — rendered output bytes for the client terminal.
//   - [MsgStderr] ([StderrMsg]) — error output bytes for the client terminal.
//
// ## File RPC
//
// Lets the server ask the client to read from or write to a file the
// client has access to. Supports load-buffer -, save-buffer -,
// display-message -p, and similar features without fd-passing.
//
//   - [MsgReadOpen]   ([ReadOpenMsg])   — server asks client to open a file for reading.
//   - [MsgRead]       ([FileReadMsg])   — client sends a chunk of file data.
//   - [MsgReadDone]   ([ReadDoneMsg])   — client signals end of file data.
//   - [MsgWriteOpen]  ([WriteOpenMsg])  — server asks client to open a file for writing.
//   - [MsgWrite]      ([FileWriteMsg])  — server sends a chunk of file data to write.
//   - [MsgWriteReady] ([WriteReadyMsg]) — client signals it is ready to receive WRITE.
//   - [MsgWriteClose] ([WriteCloseMsg]) — server signals end of write sequence.
//
// # Versioning
//
// The first message on any connection is [MsgVersion]. Incompatible
// versions are rejected with [MsgExit] before any other exchange.
//
// # Non-goals
//
// Not a streaming RPC framework. Not an IDL. Not pluggable. Message
// types are a closed enum defined in this package.
package proto
