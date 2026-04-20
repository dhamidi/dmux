package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "last-pane",
		Alias: []string{},
		Args:  command.ArgSpec{Flags: []string{"d", "e", "Z"}},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runLastPane,
	})
}

func runLastPane(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("last-pane: no mutator available")
	}
	win := ctx.Target.Window
	if win.LastPaneID == 0 {
		return command.Errorf("last-pane: no last pane")
	}
	if err := ctx.Mutator.SelectPane(ctx.Target.Session.ID, win.ID, win.LastPaneID); err != nil {
		return command.Errorf("last-pane: %v", err)
	}
	return command.OK()
}
