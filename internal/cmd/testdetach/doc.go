// Package testdetach implements the test-detach test-only command.
//
// Builds only under the `dmuxtest` build tag.
//
// # Synopsis
//
//	test-detach <handle>
//
// Cleanly disconnects a synthetic client. Sends a Bye frame; the
// server responds with Exit{Detached}; the synthetic client
// goroutine exits.
//
// # Typed args
//
//	type Args struct {
//	    Handle string  // positional: =A, =B
//	}
//
// # Behaviour
//
//  1. Look up the synthetic client by handle. Fail if unknown.
//  2. Send a proto.Bye frame on the connection.
//  3. Wait for the Exit frame from the server.
//  4. Close the connection.
//  5. Remove the handle from the harness's client table.
//  6. Return cmd.Ok.
//
// # Why not just `detach-client -t =A`?
//
// `detach-client` runs server-side and targets an attached client
// BY the server's client list. It's the equivalent of the user
// pressing prefix-d — the server initiates the detach.
//
// `test-detach` simulates the client disconnecting from its end —
// user closes the terminal window, kills dmux with SIGHUP, network
// drops. The two have different code paths in the server:
//
//	detach-client   server decides, sends Exit, closes socket
//	test-detach     client closes socket, server notices via reader
//	                goroutine's read error
//
// Testing both paths matters. detach-client tests the server's
// active-detach code; test-detach tests the passive-disconnect
// code.
//
// For scenarios testing user-initiated detach via prefix-d (M2-2),
// write `at =A "\x02d"` instead.
//
// # Ungraceful disconnect
//
// For scenarios testing "what if the network drops", use
// `test-detach` with `-H` (hard):
//
//	test-detach =A -H
//
// This closes the socket without sending Bye, simulating an
// unclean disconnect. The server sees read error on the connection
// and synthesizes Exit{Lost}.
//
// # Typed args, full
//
//	type Args struct {
//	    Handle string  // positional
//	    Hard   bool    // -H
//	}
//
// # Registration
//
//	var Cmd = cmd.New("test-detach", nil, exec)
//
//	func init() { cmd.Register(Cmd) }
//
// # Scope boundary
//
// Affects only the synthetic client named. No server-side cleanup
// beyond what the server does in response to a real client
// disconnect.
package testdetach
