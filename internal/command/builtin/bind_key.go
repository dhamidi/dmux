package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "bind-key",
		Alias: []string{"bind"},
		Args: command.ArgSpec{
			Flags:   []string{"n", "r"},
			Options: []string{"T", "N"},
			MinArgs: 2,
			MaxArgs: -1,
		},
		Run: runBindKey,
	})
}

func runBindKey(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("bind-key: no mutator available")
	}
	table := ctx.Args.Option("T")
	if table == "" {
		if ctx.Args.Flag("n") {
			table = "root"
		} else {
			table = "prefix"
		}
	}
	key := ctx.Args.Positional[0]
	cmd := ctx.Args.Positional[1]
	if err := ctx.Mutator.BindKey(table, key, cmd); err != nil {
		return command.Errorf("bind-key: %v", err)
	}
	return command.OK()
}
