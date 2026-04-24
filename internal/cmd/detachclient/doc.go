// Package detachclient implements the detach-client command.
//
// detach-client is the user-facing, tmux-compatible command that
// tells an attached client to leave its session. Invoked without
// arguments from inside an attached session, it detaches the calling
// client — the same effect a user gets from the default prefix-d
// binding. The server sends the client a proto.Exit frame with
// reason proto.ExitDetached and then closes the connection; the
// client exits cleanly and the user's terminal returns to the shell
// that ran dmux.
//
// This command is distinct from the "client" ensemble
// (internal/cmd/client), which is scenario tooling for driving
// synthetic in-process clients by name. `client kill` tears a
// synthetic client down from the outside; `detach-client` asks the
// server to detach a real attached client from the inside. The two
// address different layers and must not be conflated.
//
// # Synopsis
//
//	detach-client
//
// Milestone one accepts no flags and no arguments: the command
// detaches the calling client only.
//
// # Typed args
//
//	type Args struct {
//	    Target     string `dmux:"t=target-client"  help:"detach the named client"`
//	    Session    string `dmux:"s=target-session" help:"detach every client on the session"`
//	    AllExcept  bool   `dmux:"a"                help:"with -s, detach every client except the caller"`
//	    Parent     bool   `dmux:"P"                help:"send SIGHUP to the client process instead of closing"`
//	    ShellExec  string `dmux:"E=shell-command"  help:"run shell-command on the client before detaching"`
//	}
//
// No flags are implemented in milestone one.
//
// Deferred (not on the struct yet): Target, Session, AllExcept,
// Parent, ShellExec. Target and Session both require a server-side
// client registry that M1 does not expose; Parent and ShellExec
// require additional exit-path plumbing that is out of scope for the
// walking skeleton.
//
// # Behavior
//
//  1. Read the calling client off the Item. The M1 invocation always
//     originates from the very connection that will be detached;
//     there is no target resolution to perform.
//  2. Record the detach intent on the Item by calling
//     SetDetach(proto.ExitDetached, "detach-client"). The command
//     does not itself send a frame; the server reads the recorded
//     intent after the queue drains and emits the Exit frame along
//     the normal exit path.
//  3. Return cmd.Ok. The successful Result is the signal to the
//     server that "this connection should exit with the recorded
//     reason once the queue finishes" rather than "enter pump
//     against an attach target."
//
// # Attachment semantics
//
// detach-client is the inverse of attach-session: attach-session
// records a SessionRef on the Item for the server to enter pump
// against after the queue drains, detach-client records an
// ExitReason for the server to emit and then close. The two paths
// are mutually exclusive within a single command invocation — a
// successful Exec sets at most one of attach-target or detach-intent,
// and the server picks the corresponding post-drain action.
//
// Matches tmux: a detached client is fully disconnected. It does not
// linger in the session's client list, its key-table state and
// render frame cache are dropped, and its socket is closed. Other
// clients attached to the same session are unaffected.
//
// # Server integration
//
// The implementation requires one narrow addition to the cmd.Item
// interface, symmetric to SetAttachTarget:
//
//	SetDetach(reason proto.ExitReason, message string)
//
// The server's serverItem records the pair on the per-connection
// command context. After the queue drains, the server checks for a
// recorded detach intent before considering attach: if present, it
// writes proto.Exit{Reason: reason, Message: message} and closes
// the connection; otherwise it falls through to the existing
// attach-or-exit logic. A nil-or-unset detach intent preserves
// current behavior for every other command.
//
// # Exit reasons
//
// Milestone one emits proto.ExitDetached on the calling client.
// When -t (deferred) lands, the targeted other client receives
// proto.ExitDetachedOther; the caller itself still sees a normal
// CommandResult because its own connection is not detached. The
// -s (deferred) path fans proto.ExitDetachedOther out to every
// matching client; with -a the caller is excluded from the fan-out.
//
// # Error sentinels
//
// The M1 no-arg case introduces no new failure modes: there is no
// target to resolve, no argument to validate, and the calling client
// is always known. Exec returns cmd.Ok unconditionally.
//
// TODO(m2:detach-target): once -t and -s land, resolve targets
// through a ClientLookup capability on Item. Unknown clients wrap
// cmd.ErrNotFound; syntactically-ill-formed target specs wrap
// cmd.ErrInvalidTarget. Both sentinels are already defined in
// internal/cmd.
//
// # Registration
//
//	const Name = "detach-client"
//
//	type command struct{}
//
//	func (command) Name() string { return Name }
//	func (command) Exec(item cmd.Item, _ []string) cmd.Result {
//	    item.SetDetach(proto.ExitDetached, Name)
//	    return cmd.Ok()
//	}
//
//	func init() { cmd.Register(command{}) }
//
// # Corresponding tmux code
//
// cmd-detach-client.c.
package detachclient
