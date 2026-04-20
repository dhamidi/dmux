package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "set-option",
		Alias: []string{"set"},
		Args: command.ArgSpec{
			Flags:   []string{"a", "F", "g", "o", "q", "s", "u", "w"},
			Options: []string{"p", "t"},
			MinArgs: 1,
			MaxArgs: 2,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runSetOption,
	})
}

func runSetOption(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("set-option: no mutator available")
	}

	name := ctx.Args.Positional[0]
	value := ""
	if len(ctx.Args.Positional) > 1 {
		value = ctx.Args.Positional[1]
	}

	scope := "session"
	if ctx.Args.Flag("g") {
		scope = "global"
	} else if ctx.Args.Flag("s") {
		scope = "server"
	} else if ctx.Args.Flag("w") {
		scope = "window"
	}

	if ctx.Args.Flag("u") {
		if err := ctx.Mutator.UnsetOption(scope, name); err != nil {
			return command.Errorf("set-option: %v", err)
		}
		return command.OK()
	}

	if err := ctx.Mutator.SetOption(scope, name, value); err != nil {
		return command.Errorf("set-option: %v", err)
	}
	return command.OK()
}
