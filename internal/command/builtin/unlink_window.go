package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "unlink-window",
		Alias: []string{"unlinkw"},
		Args: command.ArgSpec{
			Flags: []string{"k"},
		},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runUnlinkWindow,
	})
}

func runUnlinkWindow(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("unlink-window: no mutator available")
	}
	kill := ctx.Args.Flag("k")
	if err := ctx.Mutator.UnlinkWindow(ctx.Target.Session.ID, ctx.Target.Window.ID, kill); err != nil {
		return command.Errorf("unlink-window: %v", err)
	}
	return command.OK()
}
