package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "send-keys",
		Alias: []string{"send"},
		Args: command.ArgSpec{
			Flags:   []string{"H", "l", "M", "R", "X"},
			Options: []string{"N"},
			MinArgs: 1,
			MaxArgs: -1,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetPane,
			Optional: true,
		},
		Run: runSendKeys,
	})
}

func runSendKeys(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("send-keys: no mutator available")
	}
	if err := ctx.Mutator.SendKeys(ctx.Target.Pane.ID, ctx.Args.Positional); err != nil {
		return command.Errorf("send-keys: %v", err)
	}
	return command.OK()
}
