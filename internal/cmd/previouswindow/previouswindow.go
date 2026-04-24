package previouswindow

import "github.com/dhamidi/dmux/internal/cmd"

// Name is the canonical command name.
const Name = "previous-window"

// command is the zero-struct implementing cmd.Command for
// previous-window. Symmetric to next-window: rewinds the caller's
// current session's window cursor by one, wrapping from the first
// window back to the last. Matches tmux's default prefix-p binding
// behaviour once bindings land in a later subagent.
//
// M1 scope: no flags. The tmux command-line accepts -a (rewind
// to the previous window with an alert) and -t (target session);
// both are deferred.
type command struct{}

// Name returns the registered command name.
func (command) Name() string { return Name }

// Exec rewinds the caller's current session's window cursor by -1
// through Item.AdvanceWindow. A nil session (handshake-path
// invocation or detached connection) returns ErrNotFound; the
// server resolves the ref and, if the session has no windows,
// surfaces its own ErrNotFound wrap.
func (command) Exec(item cmd.Item, _ []string) cmd.Result {
	sess := item.CurrentSession()
	if sess == nil {
		return cmd.Err(cmd.ErrNotFound)
	}
	if _, err := item.AdvanceWindow(sess, -1); err != nil {
		return cmd.Err(err)
	}
	return cmd.Ok()
}

func init() {
	cmd.Register(command{})
}
