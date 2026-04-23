package attachsession

import "github.com/dhamidi/dmux/internal/cmd"

// Name is the canonical command name.
const Name = "attach-session"

// command is the zero-struct implementing cmd.Command for
// attach-session. It picks a target session (M1: MostRecent — -t
// lookup lands in M2), records it on the Item, and returns Ok. The
// server interprets a successful Result as "enter pump against the
// recorded target."
type command struct{}

// Name returns the registered command name.
func (command) Name() string { return Name }

// Exec resolves the default target (MostRecent). Returns
// ErrNotFound when the registry is empty — matches tmux's "no
// sessions" error on bare `tmux attach`.
//
// TODO(m1:attach-target): once -t session targeting lands, parse
// the flag here and call Sessions().Find instead.
func (command) Exec(item cmd.Item, _ []string) cmd.Result {
	ref := item.Sessions().MostRecent()
	if ref == nil {
		return cmd.Err(cmd.ErrNotFound)
	}
	item.SetAttachTarget(ref)
	return cmd.Ok()
}

func init() {
	cmd.Register(command{})
}
