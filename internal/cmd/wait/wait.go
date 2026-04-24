package wait

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmd/args"
	"github.com/dhamidi/dmux/internal/record"
)

// Name is the canonical command name.
const Name = "wait"

// DefaultTimeout is the fallback timeout when `-T` is not supplied.
// Scenarios that need longer waits override it explicitly; five
// seconds is chosen to make a stalled scenario fail fast in CI but
// still accommodate a cold spawn on a loaded machine.
const DefaultTimeout = 5 * time.Second

// ErrTimeout is the sentinel returned when the waited-for event does
// not fire before the timeout expires. Scenario runners dispatch on
// this via errors.Is to distinguish "wait gave up" from a
// command-level failure (bad argv, closed recorder, etc.).
var ErrTimeout = errors.New("wait: timeout")

// command is the registered wait command. It carries no state:
// everything it needs comes in via cmd.Item (context, recorder
// subscription) or argv (event name, text predicate, timeout).
type command struct{}

// Name returns the registered command name.
func (command) Name() string { return Name }

// Exec subscribes to the recorder, blocks until a matching event
// arrives or the timeout fires, and returns the corresponding Result.
//
// Argv shape:
//
//	wait <event> [--] [text] [-T duration]
//
// The `-T` flag is extracted from anywhere in argv before positional
// parsing so scenarios can write it at either end of the line. The
// optional text predicate is a Go-quoted string literal; it is
// matched as a substring against the event's `text` field.
func (command) Exec(item cmd.Item, argv []string) cmd.Result {
	rest, timeoutStr, err := extractTimeoutFlag(argv[1:])
	if err != nil {
		return cmd.Err(err)
	}
	timeout := DefaultTimeout
	if timeoutStr != "" {
		d, derr := time.ParseDuration(timeoutStr)
		if derr != nil {
			return cmd.Err(&args.ParseError{
				Phase: "flags",
				Name:  "T",
				Value: timeoutStr,
				Err:   derr,
			})
		}
		timeout = d
	}

	s := args.New(Name)
	event := s.StringArg("event", "", "recorder event name")
	text := s.StringArg("text", "", "Go-quoted substring for the event's text field")
	if err := s.Parse(rest); err != nil {
		return cmd.Err(err)
	}
	if *event == "" {
		return cmd.Err(&args.ParseError{
			Phase: "positional",
			Name:  "event",
			Err:   errors.New("event required"),
		})
	}

	var needle string
	if *text != "" {
		n, err := strconv.Unquote("\"" + *text + "\"")
		if err != nil {
			return cmd.Err(&args.ParseError{
				Phase: "positional",
				Name:  "text",
				Value: *text,
				Err:   err,
			})
		}
		needle = n
	}

	name := *event
	filter := func(ev record.Event) bool {
		if ev.Name != name {
			return false
		}
		if needle == "" {
			return true
		}
		return textContains(ev, needle)
	}

	parent := item.Context()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	ch := record.Subscribe(ctx, filter)
	defer record.Unsubscribe(ch)

	select {
	case _, ok := <-ch:
		if !ok {
			return cmd.Err(fmt.Errorf("%w: event %q did not fire within %s",
				ErrTimeout, name, timeout))
		}
		return cmd.Ok()
	case <-ctx.Done():
		return cmd.Err(fmt.Errorf("%w: event %q did not fire within %s",
			ErrTimeout, name, timeout))
	}
}

// extractTimeoutFlag pulls `-T <value>` out of argv and returns the
// remaining tokens plus the flag value (empty when absent). Supports
// both `-T 5s` and `-T=5s`. Stops scanning at `--` so a literal `-T`
// in the text predicate is not misread. A trailing `-T` with no value
// surfaces as a *args.ParseError on the flag phase so callers see the
// same diagnostic shape as any other flag parse failure.
func extractTimeoutFlag(argv []string) ([]string, string, error) {
	out := make([]string, 0, len(argv))
	value := ""
	seen := false
	i := 0
	for i < len(argv) {
		tok := argv[i]
		if tok == "--" {
			out = append(out, argv[i:]...)
			break
		}
		if tok == "-T" {
			if i+1 >= len(argv) {
				return nil, "", &args.ParseError{
					Phase: "flags",
					Name:  "T",
					Err:   errors.New("flag needs an argument"),
				}
			}
			if seen {
				return nil, "", &args.ParseError{
					Phase: "flags",
					Name:  "T",
					Err:   errors.New("flag provided more than once"),
				}
			}
			value = argv[i+1]
			seen = true
			i += 2
			continue
		}
		if len(tok) > 3 && tok[:3] == "-T=" {
			if seen {
				return nil, "", &args.ParseError{
					Phase: "flags",
					Name:  "T",
					Err:   errors.New("flag provided more than once"),
				}
			}
			value = tok[3:]
			seen = true
			i++
			continue
		}
		out = append(out, tok)
		i++
	}
	return out, value, nil
}

// textContains reports whether ev's `text` field is a string that
// contains needle as a substring. Missing or non-string fields are a
// non-match, not an error: the filter keeps waiting for a later event
// that carries a usable text field.
func textContains(ev record.Event, needle string) bool {
	v, ok := ev.Fields["text"]
	if !ok {
		return false
	}
	s, ok := v.(string)
	if !ok {
		return false
	}
	return strings.Contains(s, needle)
}

func init() {
	cmd.Register(command{})
}
