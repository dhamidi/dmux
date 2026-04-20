package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "rotate-window",
		Alias: []string{"rotatew"},
		Args: command.ArgSpec{
			Flags: []string{"D", "U", "Z"},
		},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runRotateWindow,
	})
}

func runRotateWindow(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("rotate-window: no mutator available")
	}
	// -U rotates backward; -D (default) rotates forward.
	forward := !ctx.Args.Flag("U")
	if err := ctx.Mutator.RotateWindow(ctx.Target.Session.ID, ctx.Target.Window.ID, forward); err != nil {
		return command.Errorf("rotate-window: %v", err)
	}
	return command.OK()
}
