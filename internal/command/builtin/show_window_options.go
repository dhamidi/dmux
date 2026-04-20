package builtin

import (
	"fmt"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "show-window-options",
		Alias: []string{"showw"},
		Args: command.ArgSpec{
			Flags:   []string{"g", "q", "v"},
			Options: []string{"t"},
			MaxArgs: 1,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runShowWindowOptions,
	})
}

func runShowWindowOptions(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("show-window-options: no mutator available")
	}

	scope := "window"
	if ctx.Args.Flag("g") {
		scope = "global"
	}

	filter := ""
	if len(ctx.Args.Positional) > 0 {
		filter = ctx.Args.Positional[0]
	}

	valuesOnly := ctx.Args.Flag("v")

	entries := ctx.Mutator.ListOptions(scope)
	var sb strings.Builder
	for _, e := range entries {
		if filter != "" && e.Name != filter {
			continue
		}
		if valuesOnly {
			fmt.Fprintf(&sb, "%s\n", e.Value)
		} else {
			fmt.Fprintf(&sb, "%s %s\n", e.Name, e.Value)
		}
	}
	return command.WithOutput(sb.String())
}
