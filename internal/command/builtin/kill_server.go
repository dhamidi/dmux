package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name: "kill-server",
		Args: command.ArgSpec{MaxArgs: 0},
		Run:  runKillServer,
	})
}

func runKillServer(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("kill-server: no mutator available")
	}
	if err := ctx.Mutator.KillServer(); err != nil {
		return command.Errorf("kill-server: %v", err)
	}
	return command.OK()
}
