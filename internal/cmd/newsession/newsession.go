package newsession

import "github.com/dhamidi/dmux/internal/cmd"

// Name is the canonical command name.
const Name = "new-session"

// command is the zero-struct implementing cmd.Command for
// new-session. M1 is a walking-skeleton stub: returns Ok
// unconditionally. The server interprets a successful Result as
// "enter pump", which matches the pre-refactor behavior where any
// non-kill-server command fell through to the attach path.
//
// TODO(m1:newsession-strict): error with "session already exists"
// when the server already has a session. Today new-session from a
// second terminal silently joins the existing pane — deferred so the
// refactor stays behavior-preserving.
type command struct{}

// Name returns the registered command name.
func (command) Name() string { return Name }

// Exec returns Ok. See the strict-mode TODO on the command struct.
func (command) Exec(_ cmd.Item, _ []string) cmd.Result {
	return cmd.Ok()
}

func init() {
	cmd.Register(command{})
}
