package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "customize-mode",
		Alias: []string{"customizemode"},
		Args: command.ArgSpec{
			Flags:   []string{"Z"},
			Options: []string{"t", "F", "f"},
			MaxArgs: 0,
		},
		Target: command.TargetSpec{Kind: command.TargetPane, Optional: true},
		Run:    runCustomizeMode,
	})
}

func runCustomizeMode(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("customize-mode: no mutator available")
	}
	if err := ctx.Mutator.EnterCustomizeMode(ctx.Client.ID); err != nil {
		return command.Errorf("customize-mode: %v", err)
	}
	return command.OK()
}
