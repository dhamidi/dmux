package recorder

import (
	"errors"
	"flag"
	"fmt"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmd/args"
	"github.com/dhamidi/dmux/internal/record"
)

// Name is the canonical command name.
const Name = "recorder"

// command is the ensemble entry point. Its Exec dispatches on the
// first positional to the concrete subcommand (today: set-level);
// unknown subcommands surface as structured parse errors so callers
// see the same diagnostic shape as any other malformed argv.
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
	case "set-level":
		return execSetLevel(item, argv[1:])
	default:
		return cmd.Err(&args.ParseError{
			Phase: "positional",
			Name:  "subcommand",
			Value: argv[1],
			Err:   fmt.Errorf("unknown subcommand: want set-level"),
		})
	}
}

// execSetLevel parses a single positional level name and calls
// record.SetLevel. The actual string-to-Level mapping lives in the
// levelValue flag.Value so unknown names produce a *args.ParseError
// with Phase="positional", Name="level" — matching the diagnostic
// shape every other positional uses.
func execSetLevel(_ cmd.Item, argv []string) cmd.Result {
	s := args.New(Name + " set-level")
	var lv levelValue
	s.Positional(&flag.Flag{
		Name:     "level",
		Usage:    "recorder emission level: normal or debug",
		Value:    &lv,
		DefValue: "",
	})
	if err := s.Parse(argv[1:]); err != nil {
		return cmd.Err(err)
	}
	if !lv.set {
		return cmd.Err(&args.ParseError{
			Phase: "positional",
			Name:  "level",
			Err:   errors.New("level required"),
		})
	}
	record.SetLevel(lv.level)
	return cmd.Ok()
}

func init() {
	cmd.Register(command{})
}
