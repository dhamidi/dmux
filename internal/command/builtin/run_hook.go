package builtin

import "github.com/dhamidi/dmux/internal/command"

func init() {
	command.Register(command.Spec{
		Name: "run-hook",
		Args: command.ArgSpec{
			MinArgs: 1,
			MaxArgs: 1,
		},
		Run: runRunHook,
	})
}

func runRunHook(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("run-hook: no mutator available")
	}
	ctx.Mutator.RunHook(ctx.Args.Positional[0])
	return command.OK()
}
