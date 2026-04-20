package builtin

import "github.com/dhamidi/dmux/internal/command"

func init() {
	command.Register(command.Spec{
		Name:  "lock-server",
		Alias: []string{"lockserver"},
		Args:  command.ArgSpec{},
		Run:   runLockServer,
	})
}

func runLockServer(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("lock-server: no mutator available")
	}
	if err := ctx.Mutator.LockServer(); err != nil {
		return command.Errorf("lock-server: %v", err)
	}
	return command.OK()
}
