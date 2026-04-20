package builtin

import "github.com/dhamidi/dmux/internal/command"

func init() {
	command.Register(command.Spec{
		Name: "start-server",
		Args: command.ArgSpec{},
		Run:  runStartServer,
	})
}

// runStartServer is a no-op: the dmux binary auto-starts the server when it
// connects. If this command is executing, the server is already running.
func runStartServer(_ *command.Ctx) command.Result {
	return command.OK()
}
