package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "kill-pane",
		Alias: []string{"killp"},
		Args:  command.ArgSpec{Flags: []string{"a"}},
		Target: command.TargetSpec{
			Kind:     command.TargetPane,
			Optional: true,
		},
		Run: runKillPane,
	})
}

func runKillPane(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("kill-pane: no mutator available")
	}
	if err := ctx.Mutator.KillPane(ctx.Target.Pane.ID); err != nil {
		return command.Errorf("kill-pane: %v", err)
	}
	return command.OK()
}
