package builtin

import (
	"fmt"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "choose-buffer",
		Alias: []string{"choosebuffer"},
		Args: command.ArgSpec{
			Flags:   []string{"N", "r", "Z"},
			Options: []string{"f", "K", "O"},
			MaxArgs: 1,
		},
		Target: command.TargetSpec{Kind: command.TargetPane, Optional: true},
		Run:    runChooseBuffer,
	})
}

func runChooseBuffer(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("choose-buffer: no mutator available")
	}

	template := "paste-buffer -b '%%'"
	if len(ctx.Args.Positional) > 0 {
		template = ctx.Args.Positional[0]
	}

	bufs := ctx.Mutator.ListBuffers()

	if ctx.Args.Flag("r") {
		for i, j := 0, len(bufs)-1; i < j; i, j = i+1, j-1 {
			bufs[i], bufs[j] = bufs[j], bufs[i]
		}
	}

	noPreview := ctx.Args.Flag("N")

	items := make([]command.ChooserItem, len(bufs))
	for i, b := range bufs {
		items[i] = command.ChooserItem{
			Display: fmt.Sprintf("%-20s %d bytes", b.Name, b.Size),
			Value:   b.Name,
		}
	}

	windowID := ctx.Target.Window.ID
	if err := ctx.Mutator.EnterChooseBuffer(ctx.Client.ID, windowID, items, template); err != nil {
		return command.Errorf("choose-buffer: %v", err)
	}
	_ = noPreview
	return command.OK()
}
