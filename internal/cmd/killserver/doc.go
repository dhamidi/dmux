// Package killserver implements the kill-server command.
//
// # Synopsis
//
//	kill-server
//
// No flags, no arguments.
//
// # Typed args
//
//	type Args struct{}
//
// The empty Args struct is the convention for commands that take
// nothing. The args parser still validates that no extraneous
// arguments were passed.
//
// # Behavior
//
// Asks the server to shut down cleanly:
//
//  1. Cancel the server's root context. This cascades to every
//     Client context, every Pane context, every pty reader, every
//     parked Await fan-in goroutine.
//  2. The Client writer goroutines flush pending Output frames and
//     close their socket connections.
//  3. The Pane reader goroutines exit; pty.Close sends SIGHUP (Unix)
//     or CTRL_CLOSE_EVENT (Windows) to each child process. After a
//     short grace period, escalate to SIGKILL / TerminateProcess for
//     stragglers.
//  4. Close the socket listener so no new clients can connect.
//  5. server.Run returns; the server process exits.
//
// All of this is plain context-cancellation cascade. No manual
// CLIENT_DEAD flags, no carefully ordered destruction, no reference
// counts to drain. See `docs/go-patterns.md` for the cancellation
// model.
//
// Milestone one does not implement the kill-server hook (for
// user-defined on-shutdown scripts) or the exit-empty /
// exit-unattached interaction. Those are independent features.
//
// # Registration
//
//	func init() {
//	    cmd.Register(cmd.New("kill-server", nil, exec))
//	}
//
//	func exec(item *cmd.Item, _ *Args) cmd.Result {
//	    item.Server().Shutdown()
//	    return cmd.Ok
//	}
//
// # Corresponding tmux code
//
// cmd-kill-server.c plus the server_send_exit / server_loop
// termination logic in server.c.
package killserver
