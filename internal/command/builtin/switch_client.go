package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "switch-client",
		Alias: []string{"switchc"},
		Args:  command.ArgSpec{Flags: []string{"E", "l", "n", "p", "r", "Z"}},
		Target: command.TargetSpec{
			Kind:     command.TargetSession,
			Optional: true,
		},
		Run: runSwitchClient,
	})
}

func runSwitchClient(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("switch-client: no mutator available")
	}
	if err := ctx.Mutator.SwitchClient(ctx.Client.ID, ctx.Target.Session.ID); err != nil {
		return command.Errorf("switch-client: %v", err)
	}
	return command.OK()
}
