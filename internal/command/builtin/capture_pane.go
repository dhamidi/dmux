package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "capture-pane",
		Alias: []string{"capturep"},
		Args: command.ArgSpec{
			Flags:   []string{"p", "S"},
			Options: []string{"b"},
		},
		Target: command.TargetSpec{
			Kind:     command.TargetPane,
			Optional: true,
		},
		Run: runCapturePane,
	})
}

func runCapturePane(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("capture-pane: no mutator available")
	}

	history := ctx.Args.Flag("S")
	printToStdout := ctx.Args.Flag("p")
	bufName := ctx.Args.Option("b")

	content, err := ctx.Mutator.CapturePane(ctx.Target.Pane.ID, history)
	if err != nil {
		return command.Errorf("capture-pane: %v", err)
	}

	if printToStdout {
		return command.WithOutput(content)
	}

	// Store in a named buffer (or auto-named if bufName is empty).
	if err := ctx.Mutator.SetBuffer(bufName, content); err != nil {
		return command.Errorf("capture-pane: %v", err)
	}
	return command.OK()
}
