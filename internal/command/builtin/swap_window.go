package builtin

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "swap-window",
		Alias: []string{"swapw"},
		Args: command.ArgSpec{
			Options: []string{"s"},
		},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runSwapWindow,
	})
}

func runSwapWindow(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("swap-window: no mutator available")
	}
	srcRaw := ctx.Args.Option("s")
	if srcRaw == "" {
		return command.Errorf("swap-window: -s required")
	}
	srcWin, err := windowByRef(ctx.Target.Session, srcRaw)
	if err != nil {
		return command.Errorf("swap-window: %v", err)
	}
	if err := ctx.Mutator.SwapWindows(ctx.Target.Session.ID, ctx.Target.Window.ID, srcWin.ID); err != nil {
		return command.Errorf("swap-window: %v", err)
	}
	return command.OK()
}

// windowByRef finds a window in sess by ID, name, or numeric index.
// A leading colon (e.g. ":2") is stripped before matching.
func windowByRef(sess command.SessionView, ref string) (command.WindowView, error) {
	ref = strings.TrimPrefix(ref, ":")
	for _, w := range sess.Windows {
		if w.ID == ref || w.Name == ref {
			return w, nil
		}
	}
	if idx, err := strconv.Atoi(ref); err == nil {
		for _, w := range sess.Windows {
			if w.Index == idx {
				return w, nil
			}
		}
		return command.WindowView{}, fmt.Errorf("window index %d not found", idx)
	}
	return command.WindowView{}, fmt.Errorf("window %q not found", ref)
}
