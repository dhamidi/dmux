package builtin

import "github.com/dhamidi/dmux/internal/command"

func init() {
	command.Register(command.Spec{
		Name:  "set-hook",
		Alias: []string{"seth"},
		Args: command.ArgSpec{
			Flags:   []string{"g", "u", "R"},
			MinArgs: 1,
			MaxArgs: 2,
		},
		Run: runSetHook,
	})
}

func runSetHook(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("set-hook: no mutator available")
	}
	// -u flag: unset all hooks for event
	if ctx.Args.Flag("u") {
		event := ctx.Args.Positional[0]
		if err := ctx.Mutator.SetHook(event, ""); err != nil {
			return command.Result{Err: err}
		}
		return command.OK()
	}
	event := ctx.Args.Positional[0]
	cmd := ""
	if len(ctx.Args.Positional) > 1 {
		cmd = ctx.Args.Positional[1]
	}
	if err := ctx.Mutator.SetHook(event, cmd); err != nil {
		return command.Result{Err: err}
	}
	// -R flag: run the hook immediately after registering
	if ctx.Args.Flag("R") {
		ctx.Mutator.RunHook(event)
	}
	return command.OK()
}
