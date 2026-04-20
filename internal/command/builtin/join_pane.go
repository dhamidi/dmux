package builtin

import (
	"strconv"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "join-pane",
		Alias: []string{"joinp"},
		Args: command.ArgSpec{
			Flags:   []string{"b", "d", "h", "v"},
			Options: []string{"s", "l"},
		},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runJoinPane,
	})
}

func runJoinPane(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("join-pane: no mutator available")
	}
	srcRaw := ctx.Args.Option("s")
	if srcRaw == "" {
		return command.Errorf("join-pane: -s required")
	}

	// Parse "[session]:window.pane" — session part is optional.
	sessRef, rest, hasColon := strings.Cut(srcRaw, ":")
	if !hasColon {
		// No ":" — entire string is "window.pane" in current session.
		rest = sessRef
		sessRef = ""
	}

	srcSess := ctx.Target.Session
	if sessRef != "" {
		s, ok := ctx.Server.GetSession(sessRef)
		if !ok {
			s, ok = ctx.Server.GetSessionByName(sessRef)
		}
		if !ok {
			return command.Errorf("join-pane: session %q not found", sessRef)
		}
		srcSess = s
	}

	winRef, paneRef, _ := strings.Cut(rest, ".")

	// Resolve source window.
	var srcWin command.WindowView
	if winRef == "" {
		srcWin = srcSess.CurrentWindow()
	} else {
		w, err := windowByRef(srcSess, winRef)
		if err != nil {
			return command.Errorf("join-pane: %v", err)
		}
		srcWin = w
	}

	// Resolve source pane ID.
	srcPaneID := srcWin.Active
	if paneRef != "" {
		idStr := strings.TrimPrefix(paneRef, "%")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return command.Errorf("join-pane: invalid pane ref %q: %v", paneRef, err)
		}
		srcPaneID = id
	}

	if err := ctx.Mutator.JoinPane(srcSess.ID, srcWin.ID, srcPaneID, ctx.Target.Session.ID, ctx.Target.Window.ID); err != nil {
		return command.Errorf("join-pane: %v", err)
	}
	return command.OK()
}
