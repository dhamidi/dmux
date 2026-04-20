package builtin

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "list-commands",
		Alias: []string{"lscm"},
		Args:  command.ArgSpec{Flags: []string{"F"}, MaxArgs: 1},
		Run:   runListCommands,
	})
}

func runListCommands(ctx *command.Ctx) command.Result {
	filter := ""
	if len(ctx.Args.Positional) > 0 {
		filter = ctx.Args.Positional[0]
	}

	specs := command.List()
	// Sort by name for deterministic output.
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].Name < specs[j].Name
	})

	var sb strings.Builder
	for _, s := range specs {
		if filter != "" && !strings.Contains(s.Name, filter) {
			continue
		}
		if len(s.Alias) > 0 {
			fmt.Fprintf(&sb, "%s (%s)\n", s.Name, strings.Join(s.Alias, ", "))
		} else {
			fmt.Fprintf(&sb, "%s\n", s.Name)
		}
	}
	return command.WithOutput(sb.String())
}
