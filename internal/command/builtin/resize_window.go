package builtin

import (
	"strconv"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "resize-window",
		Alias: []string{"resizew"},
		Args: command.ArgSpec{
			Flags:   []string{"a", "A", "D", "U", "L", "R"},
			Options: []string{"x", "y"},
			MaxArgs: 1,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runResizeWindow,
	})
}

func runResizeWindow(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("resize-window: no mutator available")
	}
	win := ctx.Target.Window

	cols := win.Cols
	rows := win.Rows
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}

	// Explicit dimensions take precedence.
	if xStr := ctx.Args.Option("x"); xStr != "" {
		n, err := strconv.Atoi(xStr)
		if err != nil || n < 1 {
			return command.Errorf("resize-window: invalid width %q", xStr)
		}
		cols = n
	}
	if yStr := ctx.Args.Option("y"); yStr != "" {
		n, err := strconv.Atoi(yStr)
		if err != nil || n < 1 {
			return command.Errorf("resize-window: invalid height %q", yStr)
		}
		rows = n
	}

	// Directional adjustments.
	adj := 1
	if len(ctx.Args.Positional) > 0 {
		n, err := strconv.Atoi(ctx.Args.Positional[0])
		if err != nil || n < 1 {
			return command.Errorf("resize-window: invalid amount %q", ctx.Args.Positional[0])
		}
		adj = n
	}
	switch {
	case ctx.Args.Flag("D"):
		rows += adj
	case ctx.Args.Flag("U"):
		rows -= adj
		if rows < 1 {
			rows = 1
		}
	case ctx.Args.Flag("R"):
		cols += adj
	case ctx.Args.Flag("L"):
		cols -= adj
		if cols < 1 {
			cols = 1
		}
	}

	// -a: largest client, -A: smallest client.
	if ctx.Args.Flag("a") || ctx.Args.Flag("A") {
		clients := ctx.Server.ListClients()
		best := -1
		for _, c := range clients {
			if c.SessionID != ctx.Target.Session.ID {
				continue
			}
			size := c.Cols * c.Rows
			if best < 0 ||
				(ctx.Args.Flag("a") && size > best) ||
				(ctx.Args.Flag("A") && size < best) {
				cols = c.Cols
				rows = c.Rows
				best = size
			}
		}
	}

	if err := ctx.Mutator.ResizeWindow(ctx.Target.Session.ID, win.ID, cols, rows); err != nil {
		return command.Errorf("resize-window: %v", err)
	}
	return command.OK()
}
