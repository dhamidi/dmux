package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "unbind-key",
		Alias: []string{"unbind"},
		Args: command.ArgSpec{
			Flags:   []string{"a", "n"},
			Options: []string{"T"},
			MinArgs: 0,
			MaxArgs: 1,
		},
		Run: runUnbindKey,
	})
}

func runUnbindKey(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("unbind-key: no mutator available")
	}
	table := ctx.Args.Option("T")
	if table == "" {
		if ctx.Args.Flag("n") {
			table = "root"
		} else {
			table = "prefix"
		}
	}
	if len(ctx.Args.Positional) == 0 {
		return command.Errorf("unbind-key: key required")
	}
	key := ctx.Args.Positional[0]
	if err := ctx.Mutator.UnbindKey(table, key); err != nil {
		return command.Errorf("unbind-key: %v", err)
	}
	return command.OK()
}
