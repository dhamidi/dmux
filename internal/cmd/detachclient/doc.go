// Package detachclient implements the detach-client command.
//
// detach-client cleanly disconnects a synthetic client previously
// created by attach-client. It simulates the client closing its end
// of the socket — the same code path the server exercises when a
// user closes their terminal window or SIGHUPs a real dmux binary.
//
// This is distinct from any server-initiated detach (future
// detach-session, or the server's response to prefix-d): those
// originate in the server and close the connection from the server
// side. detach-client closes from the client side, letting the
// server observe the drop through its reader goroutine.
//
// # Synopsis
//
//	detach-client <handle>
//
// # Typed args
//
//	type Args struct {
//	    Handle string  // positional: =A, =B
//	    Hard   bool    // -H, close socket without sending Bye
//	}
//
// # Behaviour
//
//  1. Look up the synthetic client by handle. Fail if unknown.
//  2. Send a proto.Bye frame on the connection (unless -H).
//  3. Wait for the Exit frame from the server.
//  4. Close the connection.
//  5. Remove the handle from the client table.
//  6. Return cmd.Ok.
//
// # Why not reuse some server-side detach?
//
// A server-side detach runs inside the server's main goroutine and
// closes the connection from that end. The server picks the Exit
// reason, sends it, and tears down. detach-client instead simulates
// the passive-disconnect path: user closes terminal, network drops,
// dmux process gets SIGHUP. The two have different server code
// paths:
//
//	server-initiated   server decides, sends Exit, closes socket
//	detach-client      client closes socket, server notices via
//	                   reader goroutine's read error
//
// Both paths matter and are exercised separately.
//
// For scenarios testing user-initiated detach via prefix-d (M2-2),
// write `client at =A "\x02d"` instead — that drives the same
// server-initiated path a real user would trigger.
//
// # Ungraceful disconnect
//
// For scenarios testing "what if the network drops", use
// detach-client with -H (hard):
//
//	detach-client =A -H
//
// This closes the socket without sending Bye, simulating an unclean
// disconnect. The server sees read error on the connection and
// synthesizes Exit{Lost}.
//
// # Registration
//
//	var Cmd = cmd.New("detach-client", nil, exec)
//
//	func init() { cmd.Register(Cmd) }
//
// # Scope boundary
//
// Affects only the synthetic client named. No server-side cleanup
// beyond what the server does in response to a real client
// disconnect.
package detachclient
