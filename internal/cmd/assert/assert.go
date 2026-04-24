package assert

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmd/args"
)

// Name is the canonical command name.
const Name = "assert"

// ClientOptionPrefix is the user-option namespace the client command
// writes to when it stores a spawned client's handle. assert reads
// from the same place so `client spawn foo` and `assert screen
// contains foo ...` reference the same synthetic client.
const ClientOptionPrefix = "@client/"

// ErrAssertion is the sentinel for a failed predicate: the command
// ran to completion and the underlying state was inspected, but the
// expected relation did not hold. Scenario runners dispatch on this
// via errors.Is to distinguish "assertion fired" from "the command
// itself errored out".
//
// TODO(assert:vt-matching): raw-byte substring matching is the M1
// shape. Once a vt.Terminal is available per synthetic client the
// predicate set should grow to include glyph-level matching
// ("screen contains-line", "screen at row/col") so assertions read
// the painted grid instead of the byte stream.
var ErrAssertion = errors.New("assert: failed")

// command is the ensemble entry point. Its Exec dispatches on the
// first positional to pick an assertion kind (today only "screen");
// the selected kind dispatches again on a predicate ("contains").
// Two levels of dispatch leave room for future kinds (client,
// session) and predicates (matches, equals) without restructuring
// existing callers.
type command struct{}

// Name returns the registered command name.
func (command) Name() string { return Name }

// Exec dispatches to an assertion-kind handler based on argv[1].
func (command) Exec(item cmd.Item, argv []string) cmd.Result {
	if len(argv) < 2 {
		return cmd.Err(&args.ParseError{
			Phase: "positional",
			Name:  "kind",
			Err:   errors.New("missing assertion kind"),
		})
	}
	switch argv[1] {
	case "screen":
		return execScreen(item, argv[1:])
	default:
		return cmd.Err(&args.ParseError{
			Phase: "positional",
			Name:  "kind",
			Value: argv[1],
			Err:   errors.New("unknown assertion kind: want screen"),
		})
	}
}

// execScreen dispatches to a screen-predicate handler based on
// argv[1]. Mirrors the ensemble shape so a future "matches" predicate
// slots in as another case without disturbing the "contains" path.
func execScreen(item cmd.Item, argv []string) cmd.Result {
	if len(argv) < 2 {
		return cmd.Err(&args.ParseError{
			Phase: "positional",
			Name:  "predicate",
			Err:   errors.New("missing predicate"),
		})
	}
	switch argv[1] {
	case "contains":
		return execScreenContains(item, argv[1:])
	default:
		return cmd.Err(&args.ParseError{
			Phase: "positional",
			Name:  "predicate",
			Value: argv[1],
			Err:   errors.New("unknown predicate: want contains"),
		})
	}
}

// execScreenContains parses argv, looks up the client handle stored
// in @client/<name>, fetches the client's accumulated screen bytes,
// and checks whether the Go-quoted text literal appears as a
// substring. A stale client reference unsets the option — mirroring
// `client at`'s behavior so repeated invocations converge — and
// surfaces the stale-ref error. A successful lookup whose screen
// does not contain the needle returns a wrapped ErrAssertion so
// scenario runners can dispatch on it.
func execScreenContains(item cmd.Item, argv []string) cmd.Result {
	s := args.New(Name + " screen contains")
	name := s.StringArg("client", "", "client handle")
	text := s.StringArg("text", "", "Go-quoted substring to find")
	if err := s.Parse(argv[1:]); err != nil {
		return cmd.Err(err)
	}
	if *name == "" {
		return cmd.Err(&args.ParseError{
			Phase: "positional",
			Name:  "client",
			Err:   errors.New("handle required"),
		})
	}
	if *text == "" {
		return cmd.Err(&args.ParseError{
			Phase: "positional",
			Name:  "text",
			Err:   errors.New("text required"),
		})
	}
	needle, err := strconv.Unquote("\"" + *text + "\"")
	if err != nil {
		return cmd.Err(&args.ParseError{
			Phase: "positional",
			Name:  "text",
			Value: *text,
			Err:   err,
		})
	}
	ref := item.Options().GetString(ClientOptionPrefix + *name)
	if ref == "" {
		return cmd.Err(cmd.ErrNotFound)
	}
	screen, err := item.Clients().Screen(ref)
	if err != nil {
		if errors.Is(err, cmd.ErrStaleClient) {
			_ = item.Options().Unset(ClientOptionPrefix + *name)
		}
		return cmd.Err(err)
	}
	if bytes.Contains(screen, []byte(needle)) {
		return cmd.Ok()
	}
	return cmd.Err(fmt.Errorf("%w: screen of %q does not contain %q (saw %d bytes)",
		ErrAssertion, *name, needle, len(screen)))
}

func init() {
	cmd.Register(command{})
}
