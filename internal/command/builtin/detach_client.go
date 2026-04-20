package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "detach-client",
		Alias: []string{"detach"},
		Args: command.ArgSpec{
			Flags:   []string{"a", "P"},
			Options: []string{"t"},
		},
		Run: runDetachClient,
	})
}

func runDetachClient(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("detach-client: no mutator available")
	}
	clientID := ctx.Args.Option("t")
	if clientID == "" {
		clientID = ctx.Client.ID
	}
	if err := ctx.Mutator.DetachClient(clientID); err != nil {
		return command.Errorf("detach-client: %v", err)
	}
	return command.OK()
}
