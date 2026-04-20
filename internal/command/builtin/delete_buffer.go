package builtin

import "github.com/dhamidi/dmux/internal/command"

func init() {
	command.Register(command.Spec{
		Name:  "delete-buffer",
		Alias: []string{"deleteb"},
		Args: command.ArgSpec{
			Options: []string{"b"},
			MaxArgs: 0,
		},
		Run: runDeleteBuffer,
	})
}

func runDeleteBuffer(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("delete-buffer: no mutator available")
	}
	name := ctx.Args.Option("b")
	if err := ctx.Mutator.DeleteBuffer(name); err != nil {
		return command.Errorf("delete-buffer: %v", err)
	}
	return command.OK()
}
