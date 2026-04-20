package builtin

import (
	"fmt"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "choose-client",
		Alias: []string{"chooseclient"},
		Args: command.ArgSpec{
			Flags:   []string{"N", "r", "Z"},
			Options: []string{"f", "K", "O"},
			MaxArgs: 1,
		},
		Target: command.TargetSpec{Kind: command.TargetPane, Optional: true},
		Run:    runChooseClient,
	})
}

func runChooseClient(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("choose-client: no mutator available")
	}

	template := "switch-client -t '%%'"
	if len(ctx.Args.Positional) > 0 {
		template = ctx.Args.Positional[0]
	}

	clients := ctx.Server.ListClients()

	if ctx.Args.Flag("r") {
		for i, j := 0, len(clients)-1; i < j; i, j = i+1, j-1 {
			clients[i], clients[j] = clients[j], clients[i]
		}
	}

	noPreview := ctx.Args.Flag("N")

	items := make([]command.ChooserItem, len(clients))
	for i, c := range clients {
		sessInfo := c.SessionID
		if sessInfo == "" {
			sessInfo = "(detached)"
		}
		items[i] = command.ChooserItem{
			Display: fmt.Sprintf("%-20s %-20s %s", c.ID, c.TTY, sessInfo),
			Value:   c.ID,
		}
	}

	windowID := ctx.Target.Window.ID
	if err := ctx.Mutator.EnterChooseClient(ctx.Client.ID, windowID, items, template); err != nil {
		return command.Errorf("choose-client: %v", err)
	}
	_ = noPreview
	return command.OK()
}
