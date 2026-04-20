package builtin

import (
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "pipe-pane",
		Alias: []string{"pipep"},
		Args: command.ArgSpec{
			Flags:   []string{"I", "O", "o"},
			MaxArgs: 1,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetPane,
			Optional: true,
		},
		Run: runPipePane,
	})
}

func runPipePane(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("pipe-pane: no mutator available")
	}

	inFlag := ctx.Args.Flag("I")
	outFlag := ctx.Args.Flag("O")
	onceFlag := ctx.Args.Flag("o")

	// If neither -I nor -O is given, default to output (-O) mode.
	if !inFlag && !outFlag {
		outFlag = true
	}

	shellCmd := ""
	if len(ctx.Args.Positional) > 0 {
		shellCmd = strings.Join(ctx.Args.Positional, " ")
	}

	if err := ctx.Mutator.PipePane(ctx.Target.Pane.ID, shellCmd, inFlag, outFlag, onceFlag); err != nil {
		return command.Errorf("pipe-pane: %v", err)
	}
	return command.OK()
}
