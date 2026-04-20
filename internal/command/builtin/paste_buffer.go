package builtin

import "github.com/dhamidi/dmux/internal/command"

func init() {
	command.Register(command.Spec{
		Name:  "paste-buffer",
		Alias: []string{"pasteb"},
		Args: command.ArgSpec{
			Flags:   []string{"d"},
			Options: []string{"b"},
			MaxArgs: 0,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetPane,
			Optional: true,
		},
		Run: runPasteBuffer,
	})
}

func runPasteBuffer(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("paste-buffer: no mutator available")
	}
	name := ctx.Args.Option("b")
	paneID := ctx.Target.Pane.ID
	if err := ctx.Mutator.PasteBuffer(name, paneID); err != nil {
		return command.Errorf("paste-buffer: %v", err)
	}
	if ctx.Args.Flag("d") {
		if err := ctx.Mutator.DeleteBuffer(name); err != nil {
			return command.Errorf("paste-buffer: delete after paste: %v", err)
		}
	}
	return command.OK()
}
