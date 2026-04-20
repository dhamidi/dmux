package builtin

import "github.com/dhamidi/dmux/internal/command"

func init() {
	command.Register(command.Spec{
		Name:  "has-session",
		Alias: []string{"has"},
		Target: command.TargetSpec{
			Kind:     command.TargetSession,
			Optional: false,
		},
		Run: runHasSession,
	})
}

// runHasSession returns exit code 0 if the named session exists (resolved by
// the framework via -t), or exit code 1 if it does not. Scripts rely on the
// exit code; no output is produced on success.
func runHasSession(_ *command.Ctx) command.Result {
	// If we reach here the framework already resolved the target session
	// successfully. Simply return OK (exit code 0).
	return command.OK()
}
