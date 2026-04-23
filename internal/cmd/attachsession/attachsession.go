package attachsession

import "github.com/dhamidi/dmux/internal/cmd"

// Name is the canonical command name.
const Name = "attach-session"

// command is the zero-struct implementing cmd.Command for
// attach-session. M1 validates session presence only — the server
// interprets a successful Result as "enter pump."
type command struct{}

// Name returns the registered command name.
func (command) Name() string { return Name }

// Exec verifies the server has a session to attach to. The actual
// pane spawn, subscription, and pump entry live on the server side;
// this Exec is purely a gate.
//
// TODO(m1:attach-target): once -t session targeting lands, parse it
// here and let Item.HasSession take a target spec. M1's one-session
// world needs only the bit predicate.
func (command) Exec(item cmd.Item, _ []string) cmd.Result {
	if !item.HasSession() {
		return cmd.Err(cmd.ErrNotFound)
	}
	return cmd.Ok()
}

func init() {
	cmd.Register(command{})
}
