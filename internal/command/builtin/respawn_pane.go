package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "respawn-pane",
		Alias: []string{"respawnp"},
		Args: command.ArgSpec{
			Flags:   []string{"k"},
			Options: []string{"e"},
		},
		Target: command.TargetSpec{
			Kind:     command.TargetPane,
			Optional: true,
		},
		Run: runRespawnPane,
	})
}

func runRespawnPane(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("respawn-pane: no mutator available")
	}

	shell := ctx.Args.Option("e")

	if err := ctx.Mutator.RespawnPane(ctx.Target.Pane.ID, shell); err != nil {
		return command.Errorf("respawn-pane: %v", err)
	}
	return command.OK()
}
