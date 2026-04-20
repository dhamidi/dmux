package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "respawn-window",
		Alias: []string{"respawnw"},
		Args: command.ArgSpec{
			Flags:   []string{"k"},
			Options: []string{"c", "e"},
			MaxArgs: 1,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runRespawnWindow,
	})
}

func runRespawnWindow(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("respawn-window: no mutator available")
	}

	shell := ctx.Args.Option("e")
	if len(ctx.Args.Positional) > 0 {
		shell = ctx.Args.Positional[0]
	}
	dir := ctx.Args.Option("c")

	if err := ctx.Mutator.RespawnWindow(ctx.Target.Session.ID, ctx.Target.Window.ID, shell, dir); err != nil {
		return command.Errorf("respawn-window: %v", err)
	}
	return command.OK()
}
