package builtin

import (
	"strconv"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "resize-pane",
		Alias: []string{"resizep"},
		Args: command.ArgSpec{
			Flags:   []string{"D", "L", "R", "U", "Z"},
			Options: []string{"x", "y"},
			MaxArgs: 1,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetPane,
			Optional: true,
		},
		Run: runResizePane,
	})
}

func runResizePane(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("resize-pane: no mutator available")
	}

	direction := ""
	switch {
	case ctx.Args.Flag("D"):
		direction = "D"
	case ctx.Args.Flag("L"):
		direction = "L"
	case ctx.Args.Flag("R"):
		direction = "R"
	case ctx.Args.Flag("U"):
		direction = "U"
	case ctx.Args.Flag("Z"):
		direction = "Z"
	}

	amount := 1
	if len(ctx.Args.Positional) > 0 {
		n, err := strconv.Atoi(ctx.Args.Positional[0])
		if err != nil {
			return command.Errorf("resize-pane: invalid amount %q: %v", ctx.Args.Positional[0], err)
		}
		amount = n
	}

	if err := ctx.Mutator.ResizePane(ctx.Target.Pane.ID, direction, amount); err != nil {
		return command.Errorf("resize-pane: %v", err)
	}
	return command.OK()
}
