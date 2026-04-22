// Package attachsession implements the attach-session command.
//
// # Synopsis
//
//	attach-session [-d] [-t target-session]
//
// # Typed args
//
//	type Args struct {
//	    DetachOthers bool   `dmux:"d"        help:"detach other clients on target session"`
//	    Target       string `dmux:"t=target" help:"target session"`
//	}
//
// Flags implemented in milestone one: DetachOthers, Target.
//
// Deferred (not on the struct yet): -c (cwd override), -E (skip
// update-environment), -f (per-attach client flags), -r (read-only),
// -x (propagate exit status).
//
// # Behavior
//
//  1. Resolve the target session. With Target set, look up by name or
//     id. Without Target, pick the most-recently-used session
//     (tmux's "best" rule). Error if no target can be found.
//  2. Verify the invoking client is identified (termcaps profile
//     known, size known). If not, return cmd.Err; this should not
//     occur for the milestone-one path because Identify is
//     synchronous.
//  3. Set the client's current session to the target; push the
//     target's current window into the client's window history.
//  4. If a.DetachOthers, cancel every other client's context that is
//     currently attached to this session (see internal/server: each
//     Client has a context.Context whose cancellation triggers clean
//     detach).
//  5. Trigger a full redraw on the invoking client by clearing its
//     per-pane termout frame cache, forcing the next render to be a
//     full repaint.
//
// # Attachment semantics
//
// Matches tmux: a client can be attached to at most one session;
// multiple clients may share a session and see the same windows, but
// each has its own active-pane tracking, key-table state, and render
// frame cache.
//
// # Registration
//
//	var Cmd = cmd.New("attach-session", []string{"attach"}, exec)
//
//	func init() { cmd.Register(Cmd) }
//
//	func exec(item *cmd.Item, a *Args) cmd.Result {
//	    s, err := resolveSession(item, a.Target)
//	    if err != nil { return cmd.Err(err) }
//	    item.Client().attachTo(s)
//	    if a.DetachOthers { detachOthersOf(s, item.Client()) }
//	    return cmd.Ok
//	}
//
// `Cmd` is exported so newsession.exec can `cmd.Run(attachsession.Cmd, ...)`
// without going through the registry by name. This keeps the
// new-session-then-attach chain a normal Go reference rather than a
// string lookup.
//
// # Corresponding tmux code
//
// cmd-attach-session.c.
package attachsession
