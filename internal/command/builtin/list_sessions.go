package builtin

import (
	"fmt"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "list-sessions",
		Alias: []string{"ls"},
		Args:  command.ArgSpec{Flags: []string{"F"}, Options: []string{"f"}},
		Run:   runListSessions,
	})
}

func runListSessions(ctx *command.Ctx) command.Result {
	sessions := ctx.Server.ListSessions()
	var sb strings.Builder
	for _, s := range sessions {
		fmt.Fprintf(&sb, "%s: %d windows", s.Name, len(s.Windows))
		if s.ID != "" {
			fmt.Fprintf(&sb, " (created %s)", s.ID)
		}
		sb.WriteByte('\n')
	}
	return command.WithOutput(sb.String())
}
