package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "display-panes",
		Alias: []string{"displayp"},
		Args: command.ArgSpec{
			Options: []string{"t", "d"},
			MaxArgs: 0,
		},
		Target: command.TargetSpec{Kind: command.TargetPane, Optional: true},
		Run:    runDisplayPanes,
	})
}

func runDisplayPanes(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("display-panes: no mutator available")
	}
	if err := ctx.Mutator.DisplayPanes(ctx.Client.ID); err != nil {
		return command.Errorf("display-panes: %v", err)
	}
	return command.OK()
}
