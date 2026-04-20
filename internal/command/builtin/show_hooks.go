package builtin

import (
	"fmt"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name: "show-hooks",
		Args: command.ArgSpec{
			Flags:   []string{"g"},
			Options: []string{"t"},
			MaxArgs: 0,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetSession,
			Optional: true,
		},
		Run: runShowHooks,
	})
}

func runShowHooks(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("show-hooks: no mutator available")
	}

	entries := ctx.Mutator.ListHooks()
	var sb strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&sb, "%s %s\n", e.Name, e.Value)
	}
	return command.WithOutput(sb.String())
}
