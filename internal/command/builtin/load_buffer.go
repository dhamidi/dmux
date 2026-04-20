package builtin

import "github.com/dhamidi/dmux/internal/command"

func init() {
	command.Register(command.Spec{
		Name:  "load-buffer",
		Alias: []string{"loadb"},
		Args: command.ArgSpec{
			Options: []string{"b"},
			MinArgs: 1,
			MaxArgs: 1,
		},
		Run: runLoadBuffer,
	})
}

func runLoadBuffer(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("load-buffer: no mutator available")
	}
	name := ctx.Args.Option("b")
	path := ctx.Args.Positional[0]
	if err := ctx.Mutator.LoadBuffer(name, path); err != nil {
		return command.Errorf("load-buffer: %v", err)
	}
	return command.OK()
}
