package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "kill-window",
		Alias: []string{"killw"},
		Args:  command.ArgSpec{Flags: []string{"a"}},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runKillWindow,
	})
}

func runKillWindow(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("kill-window: no mutator available")
	}
	if err := ctx.Mutator.KillWindow(ctx.Target.Session.ID, ctx.Target.Window.ID); err != nil {
		return command.Errorf("kill-window: %v", err)
	}
	return command.OK()
}
