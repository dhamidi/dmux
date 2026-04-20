package builtin

import (
	"fmt"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "list-keys",
		Alias: []string{"lsk"},
		Args: command.ArgSpec{
			Flags:   []string{"1", "N"},
			Options: []string{"P", "T"},
			MaxArgs: 0,
		},
		Run: runListKeys,
	})
}

func runListKeys(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("list-keys: no mutator available")
	}
	table := ctx.Args.Option("T")
	bindings := ctx.Mutator.ListKeyBindings(table)

	var sb strings.Builder
	for _, b := range bindings {
		if table == "" || b.Table == table {
			fmt.Fprintf(&sb, "bind-key -T %s %s %s\n", b.Table, b.Key, b.Command)
		}
	}
	return command.WithOutput(sb.String())
}
