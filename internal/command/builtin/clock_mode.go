package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name: "clock-mode",
		Args: command.ArgSpec{
			Options: []string{"t"},
			MaxArgs: 0,
		},
		Target: command.TargetSpec{Kind: command.TargetPane, Optional: true},
		Run:    runClockMode,
	})
}

func runClockMode(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("clock-mode: no mutator available")
	}
	paneID := ctx.Target.Pane.ID
	if err := ctx.Mutator.EnterClockMode(ctx.Client.ID, paneID); err != nil {
		return command.Errorf("clock-mode: %v", err)
	}
	return command.OK()
}
