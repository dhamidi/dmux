package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "new-session",
		Alias: []string{"new-s"},
		Args: command.ArgSpec{
			Flags:   []string{"d", "P"},
			Options: []string{"n", "s", "x", "y"},
			MaxArgs: 0,
		},
		Run: runNewSession,
	})
}

func runNewSession(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("new-session: no mutator available")
	}
	name := ctx.Args.Option("s")
	_, err := ctx.Mutator.NewSession(name)
	if err != nil {
		return command.Errorf("new-session: %v", err)
	}
	return command.OK()
}
