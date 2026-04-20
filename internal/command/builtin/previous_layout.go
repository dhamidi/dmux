package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "previous-layout",
		Alias: []string{"prevl"},
		Args:  command.ArgSpec{},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runPreviousLayout,
	})
}

func runPreviousLayout(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("previous-layout: no mutator available")
	}
	if err := ctx.Mutator.ApplyLayout(ctx.Target.Session.ID, ctx.Target.Window.ID, "prev"); err != nil {
		return command.Errorf("previous-layout: %v", err)
	}
	return command.OK()
}
