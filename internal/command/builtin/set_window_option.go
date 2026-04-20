package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "set-window-option",
		Alias: []string{"setw"},
		Args: command.ArgSpec{
			Flags:   []string{"a", "F", "o", "q", "u"},
			Options: []string{"t"},
			MinArgs: 1,
			MaxArgs: 2,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runSetWindowOption,
	})
}

func runSetWindowOption(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("set-window-option: no mutator available")
	}

	name := ctx.Args.Positional[0]
	value := ""
	if len(ctx.Args.Positional) > 1 {
		value = ctx.Args.Positional[1]
	}

	if ctx.Args.Flag("u") {
		if err := ctx.Mutator.UnsetOption("window", name); err != nil {
			return command.Errorf("set-window-option: %v", err)
		}
		return command.OK()
	}

	if err := ctx.Mutator.SetOption("window", name, value); err != nil {
		return command.Errorf("set-window-option: %v", err)
	}
	return command.OK()
}
