package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "slice-pane",
		Alias: []string{},
		Args: command.ArgSpec{
			Flags:   []string{"b", "d", "h", "v"},
			Options: []string{"l", "s"},
		},
		Target: command.TargetSpec{
			Kind:     command.TargetPane,
			Optional: true,
		},
		Run: runSlicePane,
	})
}

func runSlicePane(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("slice-pane: no mutator available")
	}

	_, err := ctx.Mutator.SlicePane(ctx.Target.Session.ID, ctx.Target.Window.ID, ctx.Target.Pane.ID)
	if err != nil {
		return command.Errorf("slice-pane: %v", err)
	}
	return command.OK()
}
