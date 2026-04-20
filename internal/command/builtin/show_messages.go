package builtin

import (
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "show-messages",
		Alias: []string{"showmsgs"},
		Args:  command.ArgSpec{},
		Run:   runShowMessages,
	})
}

func runShowMessages(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("show-messages: no mutator available")
	}
	msgs := ctx.Mutator.ShowMessages()
	return command.WithOutput(strings.Join(msgs, "\n"))
}
