package builtin

import "github.com/dhamidi/dmux/internal/command"

func init() {
	command.Register(command.Spec{
		Name:  "lock-client",
		Alias: []string{"lockc"},
		Args: command.ArgSpec{
			Options: []string{"t"},
		},
		Run: runLockClient,
	})
}

func runLockClient(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("lock-client: no mutator available")
	}
	clientID := ctx.Args.Option("t")
	if clientID == "" {
		clientID = ctx.Client.ID
	}
	if err := ctx.Mutator.LockClient(clientID); err != nil {
		return command.Errorf("lock-client: %v", err)
	}
	return command.OK()
}
