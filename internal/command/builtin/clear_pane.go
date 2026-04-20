package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "clear-pane",
		Alias: []string{"clearp"},
		Args:  command.ArgSpec{},
		Target: command.TargetSpec{
			Kind:     command.TargetPane,
			Optional: true,
		},
		Run: runClearPane,
	})
}

func runClearPane(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("clear-pane: no mutator available")
	}

	if err := ctx.Mutator.ClearPane(ctx.Target.Pane.ID); err != nil {
		return command.Errorf("clear-pane: %v", err)
	}
	return command.OK()
}
