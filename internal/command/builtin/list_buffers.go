package builtin

import (
	"fmt"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "list-buffers",
		Alias: []string{"lsb"},
		Args:  command.ArgSpec{MaxArgs: 0},
		Run:   runListBuffers,
	})
}

func runListBuffers(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.OK()
	}
	bufs := ctx.Mutator.ListBuffers()
	if len(bufs) == 0 {
		return command.OK()
	}
	var sb strings.Builder
	for _, b := range bufs {
		fmt.Fprintf(&sb, "%s: %d bytes\n", b.Name, b.Size)
	}
	return command.WithOutput(sb.String())
}
