package recorder

import "github.com/dhamidi/dmux/internal/record"

// levelValue is the flag.Value that maps the textual level names
// accepted by "recorder set-level" to record.Level constants. It is
// the single place in the tree that knows "normal" → LevelNormal
// and "debug" → LevelDebug; keeping the mapping here lets the record
// package stay transport-agnostic (no strings, no arg parsing).
//
// Unknown input returns a plain error from Set; args.Parse wraps
// that into a *args.ParseError with Phase="positional", Name="level"
// so callers dispatch on parse errors uniformly.
type levelValue struct {
	level record.Level
	set   bool
}

// Set parses v as a level name and records the result. An unknown
// name produces an error whose wording lists the accepted values so
// the ParseError chain surfaces a useful diagnostic.
func (l *levelValue) Set(v string) error {
	switch v {
	case "normal":
		l.level = record.LevelNormal
	case "debug":
		l.level = record.LevelDebug
	default:
		return &unknownLevelError{name: v}
	}
	l.set = true
	return nil
}

// String renders the currently held level. Matches flag.Value's
// contract: zero value returns the empty string so the parser does
// not treat "normal" as both the default and a real input.
func (l *levelValue) String() string {
	if l == nil || !l.set {
		return ""
	}
	switch l.level {
	case record.LevelDebug:
		return "debug"
	default:
		return "normal"
	}
}

// unknownLevelError is returned by levelValue.Set when the input
// does not match a known level. Exposed through the wrapping chain
// so callers that want the offending name can errors.As into it;
// most callers just errors.Is against *args.ParseError and log
// Error().
type unknownLevelError struct{ name string }

func (e *unknownLevelError) Error() string {
	return "recorder: unknown level " + e.name + ": want normal or debug"
}
