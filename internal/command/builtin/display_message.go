package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "display-message",
		Alias: []string{"display"},
		Args: command.ArgSpec{
			Flags:   []string{"a", "I", "N", "p", "v"},
			Options: []string{"c", "d", "F", "l", "t"},
			MaxArgs: 1,
		},
		Run: runDisplayMessage,
	})
}

func runDisplayMessage(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("display-message: no mutator available")
	}
	msg := ""
	if len(ctx.Args.Positional) > 0 {
		msg = ctx.Args.Positional[0]
	}
	clientID := ctx.Args.Option("c")
	if clientID == "" {
		clientID = ctx.Client.ID
	}
	if err := ctx.Mutator.DisplayMessage(clientID, msg); err != nil {
		return command.Errorf("display-message: %v", err)
	}
	return command.OK()
}
