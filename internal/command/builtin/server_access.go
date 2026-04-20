package builtin

import "github.com/dhamidi/dmux/internal/command"

func init() {
	command.Register(command.Spec{
		Name: "server-access",
		Args: command.ArgSpec{
			Flags:   []string{"a", "d", "n", "w"},
			MaxArgs: 1,
		},
		Run: runServerAccess,
	})
}

// runServerAccess controls which users may connect to the dmux server socket.
//
// Flags:
//   - -a: allow the named user to connect.
//   - -d: deny the named user.
//   - -n: deny all new connections (no user argument required).
//   - -w: grant write access in addition to connect access (used with -a).
//
// The ACL is recorded on the server state. On a single-user system this
// minimal implementation records the policy but does not enforce it at the
// socket level. See the TODO comment in serverMutator.DenyAllClients and
// serverMutator.SetServerAccess for where enforcement would be added.
func runServerAccess(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("server-access: no mutator available")
	}

	// -n: block all new connections, no username needed.
	if ctx.Args.Flag("n") {
		if err := ctx.Mutator.DenyAllClients(); err != nil {
			return command.Errorf("server-access: %v", err)
		}
		return command.OK()
	}

	if len(ctx.Args.Positional) == 0 {
		return command.Errorf("server-access: a username argument is required (or use -n)")
	}
	username := ctx.Args.Positional[0]
	write := ctx.Args.Flag("w")

	if ctx.Args.Flag("a") {
		if err := ctx.Mutator.SetServerAccess(username, true, write); err != nil {
			return command.Errorf("server-access: %v", err)
		}
		return command.OK()
	}

	if ctx.Args.Flag("d") {
		if err := ctx.Mutator.SetServerAccess(username, false, false); err != nil {
			return command.Errorf("server-access: %v", err)
		}
		return command.OK()
	}

	return command.Errorf("server-access: specify -a (allow) or -d (deny) or -n (deny all)")
}
