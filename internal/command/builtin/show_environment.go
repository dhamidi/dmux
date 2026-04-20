package builtin

import (
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "show-environment",
		Alias: []string{"showenv"},
		Args: command.ArgSpec{
			Flags:   []string{"g", "s"},
			Options: []string{"t"},
			MinArgs: 0,
			MaxArgs: 1,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetSession,
			Optional: true,
		},
		Run: runShowEnvironment,
	})
}

func runShowEnvironment(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("show-environment: no mutator available")
	}

	scope := ctx.Target.Session.ID
	if ctx.Args.Flag("g") {
		scope = "global"
	}

	entries := ctx.Mutator.ListEnvironment(scope)

	filter := ""
	if len(ctx.Args.Positional) > 0 {
		filter = ctx.Args.Positional[0]
	}

	shellFmt := ctx.Args.Flag("s")

	var lines []string
	for _, e := range entries {
		if filter != "" && e.Name != filter {
			continue
		}
		if e.Removed {
			if shellFmt {
				lines = append(lines, "unset "+e.Name)
			} else {
				lines = append(lines, "-"+e.Name)
			}
		} else {
			if shellFmt {
				lines = append(lines, "export "+e.Name+"="+e.Value)
			} else {
				lines = append(lines, e.Name+"="+e.Value)
			}
		}
	}

	return command.WithOutput(strings.Join(lines, "\n"))
}
