package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "attach-session",
		Alias: []string{"attach", "a"},
		Args:  command.ArgSpec{Flags: []string{"d", "r", "x"}},
		Target: command.TargetSpec{
			Kind:     command.TargetSession,
			Optional: true,
		},
		Run: runAttachSession,
	})
}

func runAttachSession(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("attach-session: no mutator available")
	}
	if err := ctx.Mutator.AttachClient(ctx.Client.ID, ctx.Target.Session.ID); err != nil {
		return command.Errorf("attach-session: %v", err)
	}
	return command.OK()
}
