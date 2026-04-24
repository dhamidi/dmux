package client

import (
	"errors"
	"fmt"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmd/args"
	"github.com/dhamidi/dmux/internal/options"
)

// Name is the canonical command name.
const Name = "client"

// OptionPrefix is prepended to the per-client handle to form the
// user-option key. `client spawn foo` writes to @client/foo; `client
// kill foo` reads from the same place.
const OptionPrefix = "@client/"

// command is the ensemble entry point. Its Exec dispatches on the
// first positional to spawn or kill; unknown subcommands surface as
// parse errors so callers see structured diagnostics instead of a
// silent no-op.
type command struct{}

// Name returns the registered command name.
func (command) Name() string { return Name }

// Exec dispatches to a subcommand based on argv[1].
func (command) Exec(item cmd.Item, argv []string) cmd.Result {
	if len(argv) < 2 {
		return cmd.Err(&args.ParseError{
			Phase: "positional",
			Name:  "subcommand",
			Err:   errors.New("missing subcommand"),
		})
	}
	switch argv[1] {
	case "spawn":
		return execSpawn(item, argv[1:])
	case "kill":
		return execKill(item, argv[1:])
	default:
		return cmd.Err(&args.ParseError{
			Phase: "positional",
			Name:  "subcommand",
			Value: argv[1],
			Err:   fmt.Errorf("unknown subcommand: want spawn or kill"),
		})
	}
}

// execSpawn parses the spawn subcommand's argv, asks the
// ClientManager for a fresh client, and records the returned handle
// in the @client/<name> user option. If the option write fails, the
// spawned client is torn down so we do not leak untracked clients.
func execSpawn(item cmd.Item, argv []string) cmd.Result {
	s := args.New(Name + " spawn")
	profile := s.String("F", "", "client profile")
	cols := s.Int("x", 0, "initial columns")
	rows := s.Int("y", 0, "initial rows")
	name := s.StringArg("name", "", "client handle")
	if err := s.Parse(argv[1:]); err != nil {
		return cmd.Err(err)
	}
	if *name == "" {
		return cmd.Err(&args.ParseError{
			Phase: "positional",
			Name:  "name",
			Err:   errors.New("handle required"),
		})
	}
	ref, err := item.Clients().Spawn(*profile, *cols, *rows)
	if err != nil {
		return cmd.Err(err)
	}
	if err := item.Options().Set(OptionPrefix+*name, options.NewString(ref)); err != nil {
		_ = item.Clients().Kill(ref)
		return cmd.Err(err)
	}
	return cmd.Ok()
}

// execKill parses the kill subcommand's argv, looks up the stored
// handle, unsets the option eagerly, and tears the client down.
// Stale references (Kill returns an error wrapping ErrStaleClient)
// are treated as success: the option is already gone, the caller's
// intent is satisfied.
func execKill(item cmd.Item, argv []string) cmd.Result {
	s := args.New(Name + " kill")
	name := s.StringArg("name", "", "client handle")
	if err := s.Parse(argv[1:]); err != nil {
		return cmd.Err(err)
	}
	if *name == "" {
		return cmd.Err(&args.ParseError{
			Phase: "positional",
			Name:  "name",
			Err:   errors.New("handle required"),
		})
	}
	ref := item.Options().GetString(OptionPrefix + *name)
	if ref == "" {
		return cmd.Ok()
	}
	if err := item.Options().Unset(OptionPrefix + *name); err != nil {
		return cmd.Err(err)
	}
	if err := item.Clients().Kill(ref); err != nil && !errors.Is(err, cmd.ErrStaleClient) {
		return cmd.Err(err)
	}
	return cmd.Ok()
}

func init() {
	cmd.Register(command{})
}
