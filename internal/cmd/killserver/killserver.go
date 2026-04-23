package killserver

import "github.com/dhamidi/dmux/internal/cmd"

// Name is the canonical command name.
const Name = "kill-server"

// command is the zero-struct implementing cmd.Command for
// kill-server. It carries no flags — kill-server in tmux accepts
// nothing.
type command struct{}

// Name returns the registered command name.
func (command) Name() string { return Name }

// Exec asks the server to shut down. The Item's Shutdown
// implementation stores "kill-server" as the Exit reason's Message
// field (see internal/server.serverItem.Shutdown).
func (command) Exec(item cmd.Item, _ []string) cmd.Result {
	item.Shutdown(Name)
	return cmd.Ok()
}

func init() {
	cmd.Register(command{})
}
