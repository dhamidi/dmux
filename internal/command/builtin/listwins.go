package builtin

import (
	"fmt"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "list-windows",
		Alias: []string{"lsw"},
		Args:  command.ArgSpec{Flags: []string{"a", "F"}},
		Target: command.TargetSpec{
			Kind:     command.TargetSession,
			Optional: true,
		},
		Run: runListWindows,
	})
}

func runListWindows(ctx *command.Ctx) command.Result {
	sess := ctx.Target.Session
	var sb strings.Builder
	for _, w := range sess.Windows {
		active := ""
		if w.Index == sess.Current {
			active = "*"
		}
		fmt.Fprintf(&sb, "%d: %s%s (%d panes)\n", w.Index, w.Name, active, len(w.Panes))
	}
	return command.WithOutput(sb.String())
}
