package builtin

import (
	"strconv"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "display-popup",
		Alias: []string{"popup"},
		Args: command.ArgSpec{
			Flags:   []string{"d", "E", "B"},
			Options: []string{"t", "w", "h", "x", "y", "T"},
			MinArgs: 0,
			MaxArgs: -1,
		},
		Target: command.TargetSpec{Kind: command.TargetPane, Optional: true},
		Run:    runDisplayPopup,
	})
}

func runDisplayPopup(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("display-popup: no mutator available")
	}
	title := ctx.Args.Option("T")
	colsStr := ctx.Args.Option("w")
	rowsStr := ctx.Args.Option("h")

	cols := 80
	rows := 24
	if colsStr != "" {
		if n, err := strconv.Atoi(colsStr); err == nil {
			cols = n
		}
	}
	if rowsStr != "" {
		if n, err := strconv.Atoi(rowsStr); err == nil {
			rows = n
		}
	}

	shellCmd := strings.Join(ctx.Args.Positional, " ")
	if err := ctx.Mutator.DisplayPopup(ctx.Client.ID, shellCmd, title, cols, rows); err != nil {
		return command.Errorf("display-popup: %v", err)
	}
	return command.OK()
}
