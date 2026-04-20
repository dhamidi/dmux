package builtin

import "github.com/dhamidi/dmux/internal/command"

func init() {
	command.Register(command.Spec{
		Name:  "suspend-client",
		Alias: []string{"suspendc"},
		Args: command.ArgSpec{
			Options: []string{"t"},
		},
		Run: runSuspendClient,
	})
}

// runSuspendClient sends SIGTSTP to the dmux client process identified by -t
// (defaulting to the calling client), returning control to the shell that
// launched dmux. On resume (after SIGCONT) the client will redraw.
func runSuspendClient(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("suspend-client: no mutator available")
	}

	clientID := ctx.Args.Option("t")
	if clientID == "" {
		clientID = ctx.Client.ID
	}

	if err := ctx.Mutator.SuspendClient(clientID); err != nil {
		return command.Errorf("suspend-client: %v", err)
	}
	return command.OK()
}
