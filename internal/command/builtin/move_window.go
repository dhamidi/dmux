package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "move-window",
		Alias: []string{"movew"},
		Args: command.ArgSpec{
			Flags:   []string{"d", "r", "a"},
			Options: []string{"s"},
		},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runMoveWindow,
	})
}

func runMoveWindow(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("move-window: no mutator available")
	}
	// -a means append to end; represent as -1.
	newIndex := 0
	if ctx.Args.Flag("a") {
		newIndex = -1
	}
	if err := ctx.Mutator.MoveWindow(ctx.Target.Session.ID, ctx.Target.Window.ID, newIndex); err != nil {
		return command.Errorf("move-window: %v", err)
	}
	return command.OK()
}
