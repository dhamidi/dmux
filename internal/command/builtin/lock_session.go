package builtin

import "github.com/dhamidi/dmux/internal/command"

func init() {
	command.Register(command.Spec{
		Name:  "lock-session",
		Alias: []string{"locks"},
		Args:  command.ArgSpec{},
		Target: command.TargetSpec{
			Kind:     command.TargetSession,
			Optional: true,
		},
		Run: runLockSession,
	})
}

func runLockSession(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("lock-session: no mutator available")
	}
	sessionID := ctx.Target.Session.ID
	for _, c := range ctx.Server.ListClients() {
		if c.SessionID != sessionID {
			continue
		}
		if err := ctx.Mutator.LockClient(c.ID); err != nil {
			return command.Errorf("lock-session: %v", err)
		}
	}
	return command.OK()
}
