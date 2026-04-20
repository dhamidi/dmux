package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
	"github.com/dhamidi/dmux/internal/parse"
)

func init() {
	command.Register(command.Spec{
		Name:  "if-shell",
		Alias: []string{"if"},
		Args: command.ArgSpec{
			Flags:   []string{"b", "F"},
			Options: []string{"t"},
			MinArgs: 2,
			MaxArgs: 3,
		},
		Run: runIfShell,
	})
}

func runIfShell(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("if-shell: no mutator available")
	}
	shellCmd := ctx.Args.Positional[0]
	thenCmd := ctx.Args.Positional[1]
	elseCmd := ""
	if len(ctx.Args.Positional) > 2 {
		elseCmd = ctx.Args.Positional[2]
	}

	_, err := ctx.Mutator.RunShell(shellCmd, false)
	toRun := thenCmd
	if err != nil {
		if elseCmd == "" {
			return command.OK()
		}
		toRun = elseCmd
	}

	cmds, parseErr := parse.Parse(toRun)
	if parseErr != nil {
		return command.Errorf("if-shell: %v", parseErr)
	}
	// Capture values from ctx for use in closures.
	server := ctx.Server
	mutator := ctx.Mutator
	client := ctx.Client
	queue := ctx.Queue
	for _, c := range cmds {
		c := c
		queue.EnqueueFunc(c.Name, func() {
			command.Default.Dispatch(c.Name, c.Args, server, client, queue, mutator)
		})
	}
	return command.OK()
}
