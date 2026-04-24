package detachclient

import (
	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/proto"
)

// Name is the canonical command name.
const Name = "detach-client"

// command is the zero-struct implementing cmd.Command for
// detach-client. M1 carries no flags — the command detaches only
// the calling client. It records a detach intent on the Item; the
// server reads the intent after the queue drains and emits an
// Exit{ExitDetached} frame along the normal exit path.
type command struct{}

// Name returns the registered command name.
func (command) Name() string { return Name }

// Exec records a detach intent on the calling connection. The
// server consults the intent after the queue drains and writes the
// Exit frame itself, so Exec never fails in M1 — there is no
// target to resolve and no argument to validate.
func (command) Exec(item cmd.Item, _ []string) cmd.Result {
	item.SetDetach(proto.ExitDetached, Name)
	return cmd.Ok()
}

func init() {
	cmd.Register(command{})
}
