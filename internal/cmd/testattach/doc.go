// Package testattach implements the test-attach test-only command.
//
// Builds only under the `dmuxtest` build tag.
//
// # Synopsis
//
//	test-attach <handle> [-F profile] [-x cols] [-y rows]
//
// Spawns a synthetic dmux client in the test process and attaches
// it to the server. From the server's perspective, indistinguishable
// from a real `dmux` binary connecting from the command line.
//
// # Typed args
//
//	type Args struct {
//	    Handle  string  // positional: =A, =B
//	    Profile string  // -F: Ghostty | XTermJSModern | XTermJSLegacy |
//	                    //     WindowsTerminal | Unknown
//	    Cols    int     // -x, default 80
//	    Rows    int     // -y, default 24
//	}
//
// # Behaviour
//
//  1. Dial the server's socket with net.Dial.
//  2. Spawn a synthetic-client goroutine carrying:
//     - The server-bound connection.
//     - A real termin.Parser configured for Profile.
//     - A real vt.Terminal of size Cols x Rows (for screen reads
//       used by `assert screen -t =A`).
//     - Buffered Output and screen state.
//  3. Send an Identify frame with the configured profile and size.
//  4. Register the handle in the harness's client table.
//  5. Return cmd.Ok once client.identified event fires.
//
// The synthetic client goroutine runs for the scenario's lifetime:
//
//   - Reads Output frames, feeds bytes through termin + vt, updating
//     its screen buffer.
//   - Reads CommandResult, Exit, Beep frames, logs them for the
//     harness.
//   - Responds to test-detach by sending Bye then closing.
//   - Exits cleanly on Exit frame from server.
//
// # Why "test-attach" and not a flag on "attach-session"?
//
// `attach-session` requires an already-existing tty connection on
// the server side — it binds an existing client to a session. It
// doesn't create the connection. A real dmux client does the
// connection setup in cmd/dmux, then runs attach-session. Scenarios
// need to do both, so we have a command that does both.
//
// We could have called it `new-client`, but that's already a tmux
// command with different semantics (spawns another instance). The
// `test-` prefix signals "this is scaffolding, not production."
//
// # Profile-aware
//
// Because the synthetic client runs a real termin.Parser and
// vt.Terminal, the Profile choice matters:
//
//   - Ghostty client receives KKP-encoded key sequences; its parser
//     decodes them the same way Ghostty does.
//   - WindowsTerminal client receives legacy + modifyOtherKeys
//     encoding; its parser handles that.
//
// This means scenarios can test per-profile server behaviour with
// scenarios like:
//
//	test-attach =A -F Ghostty
//	test-attach =B -F WindowsTerminal
//	# same session, capabilities mediated to most-restrictive
//
// # Registration
//
//	var Cmd = cmd.New("test-attach", nil, exec)
//
//	func init() { cmd.Register(Cmd) }
//
// # Scope boundary
//
// No session-attach side-effect. test-attach creates a client, it
// does NOT attach the client to a session — use attach-session
// afterwards. Keeping them separate mirrors the production client
// bootstrap (Identify + CommandList(new-session, attach-session))
// and avoids hiding the attach in a "convenience."
package testattach
