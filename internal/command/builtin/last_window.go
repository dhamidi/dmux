package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "last-window",
		Alias: []string{},
		Args:  command.ArgSpec{},
		Target: command.TargetSpec{
			Kind:     command.TargetSession,
			Optional: true,
		},
		Run: runLastWindow,
	})
}

func runLastWindow(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("last-window: no mutator available")
	}
	sess := ctx.Target.Session
	if sess.LastWindowID == "" {
		return command.Errorf("last-window: no last window")
	}
	if err := ctx.Mutator.SelectWindow(sess.ID, sess.LastWindowID); err != nil {
		return command.Errorf("last-window: %v", err)
	}
	return command.OK()
}
