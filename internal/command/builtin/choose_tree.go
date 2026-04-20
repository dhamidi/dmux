package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "choose-tree",
		Alias: []string{"choosetree"},
		Args: command.ArgSpec{
			Flags:   []string{"G", "s", "w"},
			Options: []string{"t", "F"},
			MaxArgs: 0,
		},
		Target: command.TargetSpec{Kind: command.TargetWindow, Optional: true},
		Run:    runChooseTree,
	})
}

func runChooseTree(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("choose-tree: no mutator available")
	}
	sessionID := ctx.Target.Session.ID
	windowID := ctx.Target.Window.ID
	if err := ctx.Mutator.EnterChooseTree(ctx.Client.ID, sessionID, windowID); err != nil {
		return command.Errorf("choose-tree: %v", err)
	}
	return command.OK()
}
