package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "run-shell",
		Alias: []string{"run"},
		Args: command.ArgSpec{
			Flags:   []string{"b"},
			Options: []string{"d", "t"},
			MinArgs: 0,
			MaxArgs: 1,
		},
		Run: runRunShell,
	})
}

func runRunShell(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("run-shell: no mutator available")
	}
	cmd := ""
	if len(ctx.Args.Positional) > 0 {
		cmd = ctx.Args.Positional[0]
	}
	background := ctx.Args.Flag("b")
	out, err := ctx.Mutator.RunShell(cmd, background)
	if err != nil {
		return command.Errorf("run-shell: %v", err)
	}
	if out != "" {
		return command.WithOutput(out)
	}
	return command.OK()
}
