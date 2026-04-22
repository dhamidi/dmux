// Package at implements the at test-only command.
//
// Builds only under the `dmuxtest` build tag.
//
// # Synopsis
//
//	at <client-handle> <bytes>
//
// Injects bytes into the named synthetic client's input stream.
// The server sees the bytes as Input frames on that client's
// connection, identical to a real `dmux` binary receiving keystrokes
// from its tty.
//
// # Typed args
//
//	type Args struct {
//	    Client string  // positional: =A, =B
//	    Bytes  string  // positional: the bytes to inject
//	}
//
// # Bytes syntax
//
// The Bytes argument is a Go-style string with escape sequences:
//
//	at =A "echo hi\n"
//	at =A "\x02d"            // Ctrl-B d
//	at =A "\x1bOA"            // up arrow, application cursor keys
//	at =A "\x1b[A"            // up arrow, legacy
//	at =A "\u00e9"            // é (UTF-8)
//
// The parser interprets `\n`, `\r`, `\t`, `\\`, `\"`, `\xNN`,
// `\uNNNN` exactly as Go does for string literals. Everything else
// is taken literally.
//
// Rationale: real users think of input in terms of byte sequences
// (Ctrl-B = 0x02, arrow up = ESC [ A), and scenarios should read
// the same way. No abstract key-event vocabulary at this layer.
//
// # Why not `send-keys`?
//
// dmux has a real `send-keys` command (M2-2) that targets a PANE
// via `-t work:0.0` and sends key events as if the application in
// that pane received them. `at` targets a CLIENT and sends bytes as
// if that client's real terminal emitted them. They are different
// operations at different layers:
//
//	send-keys  -> pane.pty.Write (skips termin, skips routing)
//	at         -> client socket -> server termin parser -> key
//	              binding lookup -> maybe pane.SendKey
//
// The second path is what real users exercise; the first is what
// scripts and bindings use. Scenarios testing user interaction use
// `at`; scenarios testing a specific pane's behaviour in isolation
// use `send-keys`.
//
// # Behaviour
//
//  1. Look up the synthetic client by handle (=A, =B). Fail if
//     unknown.
//  2. Call client.InjectBytes(bytes). This writes an Input frame on
//     the client's socket, which the server reads on that client's
//     reader goroutine exactly as it reads any other Input frame.
//  3. Return cmd.Ok synchronously. The command does not wait for
//     the bytes to be processed — follow up with `wait` for any
//     observable effect.
//
// # Scope boundary
//
// No translation, no key-name abstraction, no "type this like a
// human" mode. Bytes go verbatim. If a scenario needs Unicode
// keypresses or KKP encoding, it writes those bytes explicitly.
package at
