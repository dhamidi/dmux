package newwindow

import "github.com/dhamidi/dmux/internal/cmd"

// Name is the canonical command name.
const Name = "new-window"

// command is the zero-struct implementing cmd.Command for
// new-window. Each invocation appends a fresh window to the
// caller's current session, backed by the server's default shell
// and using the attaching client's cwd/env. The new window becomes
// the session's current window (matching tmux's behaviour of
// switching to the freshly-created window unless -d is supplied).
//
// M1 scope: no flags. The tmux command-line accepts -a, -b, -c,
// -d, -F, -k, -n, -P, -S, -t, and a shell-command positional; all
// of those are deferred. The M1 invocation takes no arguments and
// operates only on the caller's current session.
type command struct{}

// Name returns the registered command name.
func (command) Name() string { return Name }

// Exec appends a new window to the caller's current session. The
// session is read from Item.CurrentSession; a nil session means the
// connection is not attached (typical for handshake-path
// invocations), which surfaces as ErrNotFound. An empty name tells
// the server to pick the default (the shell basename) to match
// tmux's default-window-name policy.
func (command) Exec(item cmd.Item, _ []string) cmd.Result {
	sess := item.CurrentSession()
	if sess == nil {
		return cmd.Err(cmd.ErrNotFound)
	}
	if _, err := item.SpawnWindow(sess, ""); err != nil {
		return cmd.Err(err)
	}
	return cmd.Ok()
}

func init() {
	cmd.Register(command{})
}
