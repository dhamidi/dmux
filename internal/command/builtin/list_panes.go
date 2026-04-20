package builtin

import (
	"fmt"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "list-panes",
		Alias: []string{"lsp"},
		Args:  command.ArgSpec{Flags: []string{"a", "s", "F"}},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runListPanes,
	})
}

func runListPanes(ctx *command.Ctx) command.Result {
	win := ctx.Target.Window
	var sb strings.Builder
	for i, p := range win.Panes {
		active := ""
		if p.ID == win.Active {
			active = " (active)"
		}
		fmt.Fprintf(&sb, "%d: [%d]%s%s\n", i, p.ID, p.Title, active)
	}
	return command.WithOutput(sb.String())
}
