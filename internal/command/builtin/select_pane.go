package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "select-pane",
		Alias: []string{"selectp"},
		Args:  command.ArgSpec{Flags: []string{"D", "d", "e", "L", "l", "M", "m", "R", "U", "Z"}},
		Target: command.TargetSpec{
			Kind:     command.TargetPane,
			Optional: true,
		},
		Run: runSelectPane,
	})
}

func runSelectPane(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("select-pane: no mutator available")
	}
	if err := ctx.Mutator.SelectPane(ctx.Target.Session.ID, ctx.Target.Window.ID, ctx.Target.Pane.ID); err != nil {
		return command.Errorf("select-pane: %v", err)
	}
	return command.OK()
}
