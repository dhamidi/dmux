package newsession

import "github.com/dhamidi/dmux/internal/cmd"

// Name is the canonical command name.
const Name = "new-session"

// command is the zero-struct implementing cmd.Command for
// new-session. Each invocation creates a fresh session (with its
// initial window and pane) and records it as the attach target —
// matching bare `tmux`'s behavior: running `tmux` twice gives you
// two independent sessions, never auto-attach.
//
// TODO(m1:newsession-flags): parse -d (detached) and -s (name) once
// flag parsing lands. M1 always attaches and auto-names.
type command struct{}

// Name returns the registered command name.
func (command) Name() string { return Name }

// Exec creates a new auto-named session and sets it as this
// connection's attach target. Any error from the registry (pane
// spawn failed, name collision on an explicit name) propagates.
func (command) Exec(item cmd.Item, _ []string) cmd.Result {
	ref, err := item.Sessions().Create("")
	if err != nil {
		return cmd.Err(err)
	}
	item.SetAttachTarget(ref)
	return cmd.Ok()
}

func init() {
	cmd.Register(command{})
}
