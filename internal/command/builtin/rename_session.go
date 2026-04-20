package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "rename-session",
		Alias: []string{"rename-s"},
		Args: command.ArgSpec{
			MinArgs: 1,
			MaxArgs: 1,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetSession,
			Optional: true,
		},
		Run: runRenameSession,
	})
}

func runRenameSession(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("rename-session: no mutator available")
	}
	newName := ctx.Args.Positional[0]
	if err := ctx.Mutator.RenameSession(ctx.Target.Session.ID, newName); err != nil {
		return command.Errorf("rename-session: %v", err)
	}
	return command.OK()
}
