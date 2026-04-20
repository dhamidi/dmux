package builtin

import (
	"fmt"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "break-pane",
		Alias: []string{"breakp"},
		Args: command.ArgSpec{
			Flags:   []string{"d", "P"},
			Options: []string{"n", "F"},
		},
		Target: command.TargetSpec{
			Kind:     command.TargetPane,
			Optional: true,
		},
		Run: runBreakPane,
	})
}

func runBreakPane(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("break-pane: no mutator available")
	}
	win, err := ctx.Mutator.BreakPane(ctx.Target.Session.ID, ctx.Target.Window.ID, ctx.Target.Pane.ID)
	if err != nil {
		return command.Errorf("break-pane: %v", err)
	}
	if ctx.Args.Flag("P") {
		return command.WithOutput(fmt.Sprintf("%d: %s\n", win.Index, win.Name))
	}
	return command.OK()
}
