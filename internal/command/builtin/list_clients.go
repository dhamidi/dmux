package builtin

import (
	"fmt"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "list-clients",
		Alias: []string{"lsc"},
		Args:  command.ArgSpec{Flags: []string{"F"}},
		Run:   runListClients,
	})
}

func runListClients(ctx *command.Ctx) command.Result {
	clients := ctx.Server.ListClients()
	var sb strings.Builder
	for _, c := range clients {
		attached := "(not attached)"
		if c.IsAttached() {
			attached = fmt.Sprintf("(session %s)", c.SessionID)
		}
		fmt.Fprintf(&sb, "%s: %s %s\n", c.ID, c.TTY, attached)
	}
	return command.WithOutput(sb.String())
}
