package builtin

import (
	"strconv"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "move-pane",
		Alias: []string{"movep"},
		Args: command.ArgSpec{
			Flags:   []string{"b", "d", "f", "h", "v"},
			Options: []string{"l", "s"},
		},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runMovePane,
	})
}

func runMovePane(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("move-pane: no mutator available")
	}
	srcRaw := ctx.Args.Option("s")
	if srcRaw == "" {
		// Default: use the active pane of the current window.
		srcRaw = ""
	}

	// Resolve source pane: parse "[session]:window.pane" — session part is optional.
	srcSess := ctx.Target.Session
	srcWin := ctx.Target.Session.CurrentWindow()
	srcPaneID := srcWin.Active

	if srcRaw != "" {
		sessRef, rest, hasColon := strings.Cut(srcRaw, ":")
		if !hasColon {
			rest = sessRef
			sessRef = ""
		}

		if sessRef != "" {
			s, ok := ctx.Server.GetSession(sessRef)
			if !ok {
				s, ok = ctx.Server.GetSessionByName(sessRef)
			}
			if !ok {
				return command.Errorf("move-pane: session %q not found", sessRef)
			}
			srcSess = s
		}

		winRef, paneRef, _ := strings.Cut(rest, ".")

		if winRef == "" {
			srcWin = srcSess.CurrentWindow()
		} else {
			w, err := windowByRef(srcSess, winRef)
			if err != nil {
				return command.Errorf("move-pane: %v", err)
			}
			srcWin = w
		}

		srcPaneID = srcWin.Active
		if paneRef != "" {
			idStr := strings.TrimPrefix(paneRef, "%")
			id, err := strconv.Atoi(idStr)
			if err != nil {
				return command.Errorf("move-pane: invalid pane ref %q: %v", paneRef, err)
			}
			srcPaneID = id
		}
	}

	if err := ctx.Mutator.MovePane(srcSess.ID, srcWin.ID, srcPaneID, ctx.Target.Session.ID, ctx.Target.Window.ID); err != nil {
		return command.Errorf("move-pane: %v", err)
	}
	return command.OK()
}
