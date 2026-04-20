package builtin

import (
	"fmt"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "find-window",
		Alias: []string{"findw"},
		Args: command.ArgSpec{
			Flags:   []string{"C", "N", "T"},
			MinArgs: 1,
			MaxArgs: 1,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetSession,
			Optional: true,
		},
		Run: runFindWindow,
	})
}

func runFindWindow(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("find-window: no mutator available")
	}
	pattern := ctx.Args.Positional[0]
	win, err := ctx.Mutator.FindWindow(ctx.Target.Session.ID, pattern)
	if err != nil {
		return command.Errorf("find-window: %v", err)
	}
	return command.WithOutput(fmt.Sprintf("%d: %s\n", win.Index, win.Name))
}
