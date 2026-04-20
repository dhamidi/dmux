package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name: "command-prompt",
		Args: command.ArgSpec{
			Flags:   []string{"i"},
			Options: []string{"t", "p", "I", "F"},
			MaxArgs: 0,
		},
		Target: command.TargetSpec{Kind: command.TargetPane, Optional: true},
		Run:    runCommandPrompt,
	})
}

func runCommandPrompt(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("command-prompt: no mutator available")
	}
	prompt := ctx.Args.Option("p")
	initial := ctx.Args.Option("I")
	if err := ctx.Mutator.CommandPrompt(ctx.Client.ID, prompt, initial); err != nil {
		return command.Errorf("command-prompt: %v", err)
	}
	return command.OK()
}
