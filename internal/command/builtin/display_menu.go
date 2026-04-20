package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "display-menu",
		Alias: []string{"menu"},
		Args: command.ArgSpec{
			Options: []string{"t", "x", "y", "T"},
			MinArgs: 0,
			MaxArgs: -1,
		},
		Target: command.TargetSpec{Kind: command.TargetPane, Optional: true},
		Run:    runDisplayMenu,
	})
}

func runDisplayMenu(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("display-menu: no mutator available")
	}
	args := ctx.Args.Positional
	if len(args)%3 != 0 {
		return command.Errorf("display-menu: arguments must be label/key/command triples (got %d)", len(args))
	}
	items := make([]command.MenuEntry, 0, len(args)/3)
	for i := 0; i+2 < len(args); i += 3 {
		items = append(items, command.MenuEntry{
			Label:   args[i],
			Key:     args[i+1],
			Command: args[i+2],
		})
	}
	if err := ctx.Mutator.DisplayMenu(ctx.Client.ID, items); err != nil {
		return command.Errorf("display-menu: %v", err)
	}
	return command.OK()
}
