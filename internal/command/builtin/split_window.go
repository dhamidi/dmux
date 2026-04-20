package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "split-window",
		Alias: []string{"splitw"},
		Args: command.ArgSpec{
			Flags:   []string{"b", "d", "f", "h", "P", "v", "Z"},
			Options: []string{"c", "e", "l", "p"},
			MaxArgs: 0,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetPane,
			Optional: true,
		},
		Run: runSplitWindow,
	})
}

func runSplitWindow(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("split-window: no mutator available")
	}
	_, err := ctx.Mutator.SplitWindow(ctx.Target.Session.ID, ctx.Target.Window.ID)
	if err != nil {
		return command.Errorf("split-window: %v", err)
	}
	return command.OK()
}
