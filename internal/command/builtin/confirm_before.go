package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "confirm-before",
		Alias: []string{"confirm"},
		Args: command.ArgSpec{
			Flags:   []string{"y"},
			Options: []string{"t", "p"},
			MinArgs: 1,
			MaxArgs: 1,
		},
		Target: command.TargetSpec{Kind: command.TargetPane, Optional: true},
		Run:    runConfirmBefore,
	})
}

func runConfirmBefore(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("confirm-before: no mutator available")
	}
	prompt := ctx.Args.Option("p")
	cmd := ctx.Args.Positional[0]
	if err := ctx.Mutator.ConfirmBefore(ctx.Client.ID, prompt, cmd); err != nil {
		return command.Errorf("confirm-before: %v", err)
	}
	return command.OK()
}
