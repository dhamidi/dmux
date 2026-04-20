package builtin

import (
	"fmt"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "link-window",
		Alias: []string{"linkw"},
		Args: command.ArgSpec{
			Flags:   []string{"a", "b", "d", "k"},
			Options: []string{"s"},
		},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runLinkWindow,
	})
}

func runLinkWindow(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("link-window: no mutator available")
	}

	srcRaw := ctx.Args.Option("s")
	if srcRaw == "" {
		return command.Errorf("link-window: -s is required")
	}

	// Resolve the source window, which may live in a different session.
	srcSessionID, srcWindowID, err := resolveWindowRef(ctx, srcRaw)
	if err != nil {
		return command.Errorf("link-window: %v", err)
	}

	dstSessionID := ctx.Target.Session.ID

	// Destination index: use the resolved window index when -t specified a
	// window, otherwise -1 (append).
	dstIndex := -1
	if ctx.Target.Kind >= command.TargetWindow && ctx.Target.Window.ID != "" {
		dstIndex = ctx.Target.Window.Index
	}

	afterIndex := ctx.Args.Flag("a")
	beforeIndex := ctx.Args.Flag("b")
	selectWin := !ctx.Args.Flag("d")
	killExisting := ctx.Args.Flag("k")

	if err := ctx.Mutator.LinkWindow(srcSessionID, srcWindowID, dstSessionID, dstIndex, afterIndex, beforeIndex, selectWin, killExisting); err != nil {
		return command.Errorf("link-window: %v", err)
	}
	return command.OK()
}

// resolveWindowRef resolves a "[session:]window" reference to (sessionID, windowID).
// If no session prefix is present the current session in ctx is used.
func resolveWindowRef(ctx *command.Ctx, ref string) (sessionID, windowID string, err error) {
	if idx := strings.Index(ref, ":"); idx >= 0 {
		sessRef := ref[:idx]
		winRef := ref[idx+1:]

		sess, ok := ctx.Server.GetSessionByName(sessRef)
		if !ok {
			sess, ok = ctx.Server.GetSession(sessRef)
		}
		if !ok {
			return "", "", fmt.Errorf("session %q not found", sessRef)
		}
		win, err := windowByRef(sess, winRef)
		if err != nil {
			return "", "", err
		}
		return sess.ID, win.ID, nil
	}

	// No session prefix — use the current target session.
	win, err := windowByRef(ctx.Target.Session, ref)
	if err != nil {
		return "", "", err
	}
	return ctx.Target.Session.ID, win.ID, nil
}
