package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "rename-window",
		Alias: []string{"renamew"},
		Args: command.ArgSpec{
			MinArgs: 1,
			MaxArgs: 1,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runRenameWindow,
	})
}

func runRenameWindow(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("rename-window: no mutator available")
	}
	newName := ctx.Args.Positional[0]
	if err := ctx.Mutator.RenameWindow(ctx.Target.Session.ID, ctx.Target.Window.ID, newName); err != nil {
		return command.Errorf("rename-window: %v", err)
	}
	return command.OK()
}
