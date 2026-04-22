package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:    "switch-key-table",
		Alias:   []string{},
		Args:    command.ArgSpec{MinArgs: 1, MaxArgs: 1},
		Target:  command.TargetSpec{Kind: command.TargetNone},
		Run:     runSwitchKeyTable,
	})
}

func runSwitchKeyTable(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("switch-key-table: no mutator available")
	}
	tableName := ctx.Args.Positional[0]
	if err := ctx.Mutator.SwitchKeyTable(ctx.Client.ID, tableName); err != nil {
		return command.Errorf("switch-key-table: %v", err)
	}
	return command.OK()
}
