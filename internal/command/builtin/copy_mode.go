package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "copy-mode",
		Alias: []string{"copymode"},
		Args: command.ArgSpec{
			Flags:   []string{"e", "H"},
			Options: []string{"t"},
			MaxArgs: 0,
		},
		Target: command.TargetSpec{Kind: command.TargetPane, Optional: true},
		Run:    runCopyMode,
	})
}

func runCopyMode(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("copy-mode: no mutator available")
	}
	history := ctx.Args.Flag("H")
	if err := ctx.Mutator.EnterCopyMode(ctx.Client.ID, history); err != nil {
		return command.Errorf("copy-mode: %v", err)
	}
	return command.OK()
}
