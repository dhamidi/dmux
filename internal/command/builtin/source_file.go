package builtin

import (
	"os"

	"github.com/dhamidi/dmux/internal/command"
	"github.com/dhamidi/dmux/internal/parse"
)

func init() {
	command.Register(command.Spec{
		Name:  "source-file",
		Alias: []string{"source"},
		Args: command.ArgSpec{
			Flags:   []string{"n", "q"},
			MinArgs: 1,
			MaxArgs: 1,
		},
		Run: runSourceFile,
	})
}

func runSourceFile(ctx *command.Ctx) command.Result {
	path := ctx.Args.Positional[0]
	data, err := os.ReadFile(path)
	if err != nil {
		if ctx.Args.Flag("q") {
			return command.OK()
		}
		return command.Errorf("source-file: %v", err)
	}
	cmds, parseErr := parse.Parse(string(data))
	if parseErr != nil {
		return command.Errorf("source-file: %v", parseErr)
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
