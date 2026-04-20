package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "send-prefix",
		Alias: []string{},
		Args:  command.ArgSpec{Flags: []string{"2"}},
		Target: command.TargetSpec{
			Kind:     command.TargetPane,
			Optional: true,
		},
		Run: runSendPrefix,
	})
}

func runSendPrefix(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("send-prefix: no mutator available")
	}
	optName := "prefix"
	if ctx.Args.Flag("2") {
		optName = "prefix2"
	}
	var prefixStr string
	for _, o := range ctx.Mutator.ListOptions("server") {
		if o.Name == optName {
			prefixStr = o.Value
			break
		}
	}
	if prefixStr == "" {
		return command.Errorf("send-prefix: option %q not set", optName)
	}
	if err := ctx.Mutator.SendKeys(ctx.Target.Pane.ID, []string{prefixStr}); err != nil {
		return command.Errorf("send-prefix: %v", err)
	}
	return command.OK()
}
