package builtin

import "github.com/dhamidi/dmux/internal/command"

func init() {
	command.Register(command.Spec{
		Name:  "save-buffer",
		Alias: []string{"saveb"},
		Args: command.ArgSpec{
			Options: []string{"b"},
			MinArgs: 1,
			MaxArgs: 1,
		},
		Run: runSaveBuffer,
	})
}

func runSaveBuffer(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("save-buffer: no mutator available")
	}
	name := ctx.Args.Option("b")
	path := ctx.Args.Positional[0]
	if err := ctx.Mutator.SaveBuffer(name, path); err != nil {
		return command.Errorf("save-buffer: %v", err)
	}
	return command.OK()
}
