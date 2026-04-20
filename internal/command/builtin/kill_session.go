package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "kill-session",
		Alias: []string{"kill-s"},
		Args:  command.ArgSpec{Flags: []string{"a", "C"}},
		Target: command.TargetSpec{
			Kind:     command.TargetSession,
			Optional: true,
		},
		Run: runKillSession,
	})
}

func runKillSession(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("kill-session: no mutator available")
	}
	if err := ctx.Mutator.KillSession(ctx.Target.Session.ID); err != nil {
		return command.Errorf("kill-session: %v", err)
	}
	return command.OK()
}
