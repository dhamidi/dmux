package builtin

import "github.com/dhamidi/dmux/internal/command"

func init() {
	command.Register(command.Spec{
		Name:  "set-environment",
		Alias: []string{"setenv"},
		Args: command.ArgSpec{
			Flags:   []string{"g", "r"},
			Options: []string{"t"},
			MinArgs: 1,
			MaxArgs: 2,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetSession,
			Optional: true,
		},
		Run: runSetEnvironment,
	})
}

func runSetEnvironment(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("set-environment: no mutator available")
	}

	name := ctx.Args.Positional[0]
	value := ""
	if len(ctx.Args.Positional) > 1 {
		value = ctx.Args.Positional[1]
	}

	scope := ctx.Target.Session.ID
	if ctx.Args.Flag("g") {
		scope = "global"
	}

	if err := ctx.Mutator.SetEnvironment(scope, name, value, ctx.Args.Flag("r")); err != nil {
		return command.Errorf("set-environment: %v", err)
	}
	return command.OK()
}
