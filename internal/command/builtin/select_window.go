package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "select-window",
		Alias: []string{"selectw"},
		Args:  command.ArgSpec{Flags: []string{"l", "n", "p", "T"}},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runSelectWindow,
	})
}

func runSelectWindow(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("select-window: no mutator available")
	}
	if err := ctx.Mutator.SelectWindow(ctx.Target.Session.ID, ctx.Target.Window.ID); err != nil {
		return command.Errorf("select-window: %v", err)
	}
	return command.OK()
}
