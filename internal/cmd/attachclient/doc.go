// Package attachclient implements the attach-client command.
//
// attach-client creates an in-process synthetic dmux client and
// connects it to the running server. Scenarios use it to simulate
// real user connections; hook scripts and AI agents use it to drive
// dmux from inside the server process without spawning a second
// binary.
//
// This is distinct from attach-session:
//
//   - attach-session retargets an already-connected client's view
//     onto a particular session. The connection already exists; the
//     command just rewires it.
//   - attach-client creates the connection itself. It spawns the
//     synthetic client, dials the socket, sends Identify, and
//     registers a handle the rest of the scenario can refer to.
//
// # Synopsis
//
//	attach-client <handle> [-F profile] [-x cols] [-y rows]
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
//     used by `assert screen -t =A`).
//     - Buffered Output and screen state.
//  3. Send an Identify frame with the configured profile and size.
//  4. Register the handle in the client table.
//  5. Return cmd.Ok once client.identified event fires.
//
// The synthetic client goroutine runs for the handle's lifetime:
//
//   - Reads Output frames, feeds bytes through termin + vt, updating
//     its screen buffer.
//   - Reads CommandResult, Exit, Beep frames, logs them.
//   - Responds to detach-client by sending Bye then closing.
//   - Exits cleanly on Exit frame from server.
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
// This means callers can exercise per-profile server behaviour:
//
//	attach-client =A -F Ghostty
//	attach-client =B -F WindowsTerminal
//	# same session, capabilities mediated to most-restrictive
//
// # Registration
//
//	var Cmd = cmd.New("attach-client", nil, exec)
//
//	func init() { cmd.Register(Cmd) }
//
// # Scope boundary
//
// No session-attach side-effect. attach-client creates a client, it
// does NOT attach the client to a session — use attach-session
// afterwards. Keeping them separate mirrors the production client
// bootstrap (Identify + CommandList(new-session, attach-session))
// and avoids hiding the attach in a "convenience."
package attachclient
