package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "new-window",
		Alias: []string{"neww"},
		Args: command.ArgSpec{
			Flags:   []string{"a", "d", "P"},
			Options: []string{"n", "c"},
			MaxArgs: 0,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runNewWindow,
	})
}

func runNewWindow(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("new-window: no mutator available")
	}
	name := ctx.Args.Option("n")
	_, err := ctx.Mutator.NewWindow(ctx.Target.Session.ID, name)
	if err != nil {
		return command.Errorf("new-window: %v", err)
	}
	return command.OK()
}
