package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "next-window",
		Alias: []string{"next"},
		Args:  command.ArgSpec{Flags: []string{"a"}},
		Target: command.TargetSpec{
			Kind:     command.TargetSession,
			Optional: true,
		},
		Run: runNextWindow,
	})
}

func runNextWindow(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("next-window: no mutator available")
	}
	sess := ctx.Target.Session
	if len(sess.Windows) == 0 {
		return command.Errorf("next-window: no windows in session")
	}
	skipNoAlerts := ctx.Args.Flag("a")
	cur := sess.Current
	n := len(sess.Windows)
	for i := 1; i <= n; i++ {
		idx := (cur + i) % n
		w := sess.Windows[idx]
		if skipNoAlerts && !w.ActivityFlag {
			continue
		}
		if err := ctx.Mutator.SelectWindow(sess.ID, w.ID); err != nil {
			return command.Errorf("next-window: %v", err)
		}
		return command.OK()
	}
	return command.Errorf("next-window: no suitable window found")
}
