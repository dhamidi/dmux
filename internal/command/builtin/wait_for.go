package builtin

import "github.com/dhamidi/dmux/internal/command"

func init() {
	command.Register(command.Spec{
		Name: "wait-for",
		Args: command.ArgSpec{
			Flags:   []string{"S", "L", "U"},
			MinArgs: 1,
			MaxArgs: 1,
		},
		Run: runWaitFor,
	})
}

func runWaitFor(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("wait-for: no mutator available")
	}
	channel := ctx.Args.Positional[0]
	if ctx.Args.Flag("S") {
		ctx.Mutator.SignalChannel(channel)
		return command.OK()
	}
	if err := ctx.Mutator.WaitFor(channel); err != nil {
		return command.Errorf("wait-for: %v", err)
	}
	return command.OK()
}
