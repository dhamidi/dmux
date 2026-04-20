package builtin

import (
	"strconv"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "swap-pane",
		Alias: []string{"swapp"},
		Args: command.ArgSpec{
			Flags:   []string{"d", "D", "U"},
			Options: []string{"s"},
		},
		Target: command.TargetSpec{
			Kind:     command.TargetPane,
			Optional: true,
		},
		Run: runSwapPane,
	})
}

func runSwapPane(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("swap-pane: no mutator available")
	}
	srcRaw := ctx.Args.Option("s")
	if srcRaw == "" {
		return command.Errorf("swap-pane: -s required")
	}
	// Accept plain int or %N pane reference.
	idStr := strings.TrimPrefix(srcRaw, "%")
	srcPaneID, err := strconv.Atoi(idStr)
	if err != nil {
		return command.Errorf("swap-pane: invalid pane ID %q: %v", srcRaw, err)
	}
	if err := ctx.Mutator.SwapPane(ctx.Target.Session.ID, ctx.Target.Window.ID, ctx.Target.Pane.ID, srcPaneID); err != nil {
		return command.Errorf("swap-pane: %v", err)
	}
	return command.OK()
}
