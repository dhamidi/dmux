package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "clear-history",
		Alias: []string{"clearhist"},
		Args: command.ArgSpec{
			Flags: []string{"H"},
		},
		Target: command.TargetSpec{
			Kind:     command.TargetPane,
			Optional: true,
		},
		Run: runClearHistory,
	})
}

func runClearHistory(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("clear-history: no mutator available")
	}

	visibleToo := ctx.Args.Flag("H")

	if err := ctx.Mutator.ClearHistory(ctx.Target.Pane.ID, visibleToo); err != nil {
		return command.Errorf("clear-history: %v", err)
	}
	return command.OK()
}
