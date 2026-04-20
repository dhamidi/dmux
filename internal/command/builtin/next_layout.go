package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "next-layout",
		Alias: []string{"nextl"},
		Args:  command.ArgSpec{},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runNextLayout,
	})
}

func runNextLayout(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("next-layout: no mutator available")
	}
	if err := ctx.Mutator.ApplyLayout(ctx.Target.Session.ID, ctx.Target.Window.ID, "next"); err != nil {
		return command.Errorf("next-layout: %v", err)
	}
	return command.OK()
}
