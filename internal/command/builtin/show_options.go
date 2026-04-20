package builtin

import (
	"fmt"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "show-options",
		Alias: []string{"show"},
		Args: command.ArgSpec{
			Flags:   []string{"A", "g", "H", "p", "q", "s", "v", "w"},
			Options: []string{"t"},
			MaxArgs: 1,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runShowOptions,
	})
}

func runShowOptions(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("show-options: no mutator available")
	}

	scope := "session"
	if ctx.Args.Flag("g") {
		scope = "global"
	} else if ctx.Args.Flag("s") {
		scope = "server"
	} else if ctx.Args.Flag("w") {
		scope = "window"
	}

	filter := ""
	if len(ctx.Args.Positional) > 0 {
		filter = ctx.Args.Positional[0]
	}

	entries := ctx.Mutator.ListOptions(scope)
	var sb strings.Builder
	for _, e := range entries {
		if filter != "" && e.Name != filter {
			continue
		}
		fmt.Fprintf(&sb, "%s %s\n", e.Name, e.Value)
	}
	return command.WithOutput(sb.String())
}
