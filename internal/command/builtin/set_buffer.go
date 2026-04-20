package builtin

import "github.com/dhamidi/dmux/internal/command"

func init() {
	command.Register(command.Spec{
		Name:  "set-buffer",
		Alias: []string{"setb"},
		Args: command.ArgSpec{
			Options: []string{"b"},
			MinArgs: 1,
			MaxArgs: 1,
		},
		Run: runSetBuffer,
	})
}

func runSetBuffer(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("set-buffer: no mutator available")
	}
	name := ctx.Args.Option("b")
	data := ctx.Args.Positional[0]
	if err := ctx.Mutator.SetBuffer(name, data); err != nil {
		return command.Errorf("set-buffer: %v", err)
	}
	return command.OK()
}
