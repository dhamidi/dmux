package options

// Deferred surface: Key, Color, and Command Value constructors land
// with the keys / colors / cmd packages. Walk, user-defined `@`
// options, and set-option integration land in M2.

import (
	"errors"
	"fmt"
	"strings"
)

// UserOptionPrefix marks a name as a user-defined option. Names
// starting with this prefix bypass the closed Table: they are stored
// as String values on whichever scope sets them and inherit down the
// parent chain like any Table option. Unset user options read as
// empty strings, mirroring tmux's `#{@my-var}` behavior.
const UserOptionPrefix = "@"

// IsUserOption reports whether name is a user-defined option. User
// options have no Table entry; their validation rules are weaker
// (String-only, any scope).
func IsUserOption(name string) bool {
	return strings.HasPrefix(name, UserOptionPrefix)
}

// Type tags the kind of value a Table entry holds. M1 only wires the
// first four; Key, Color, and Command values exist in the enum so
// later milestones can add Table entries without reshuffling iota
// indices (Table entries are load-bearing, see package doc).
type Type int

const (
	String Type = iota
	Number
	Bool
	Choice
	Key
	Color
	Command
)

// Scope is a bitmask of which option scopes accept an option. A Table
// entry's Scope field ORs together every scope the option is valid on;
// Set checks that the Options instance's scope bit is present.
type Scope uint8

const (
	ServerScope Scope = 1 << iota
	SessionScope
	WindowScope
	PaneScope
)

// Value is the tagged-union every Options map holds. Exported shape is
// opaque — callers construct values via the String/Number/Bool/Choice
// constructors and read them back via AsString/AsNumber/AsBool or the
// typed Options getters.
type Value struct {
	typ Type
	s   string
	n   int64
	b   bool
}

// NewString wraps s as a String-typed Value.
func NewString(s string) Value { return Value{typ: String, s: s} }

// NewNumber wraps n as a Number-typed Value.
func NewNumber(n int64) Value { return Value{typ: Number, n: n} }

// NewBool wraps b as a Bool-typed Value.
func NewBool(b bool) Value { return Value{typ: Bool, b: b} }

// NewChoice wraps s as a Choice-typed Value. The caller is responsible
// for passing a string that matches one of the Table entry's Choices;
// Options.Set validates this.
func NewChoice(s string) Value { return Value{typ: Choice, s: s} }

// Type reports the tag of this Value.
func (v Value) Type() Type { return v.typ }

// AsString returns the string payload of a String or Choice value. The
// second return is false when the tag is neither.
func (v Value) AsString() (string, bool) {
	if v.typ == String || v.typ == Choice {
		return v.s, true
	}
	return "", false
}

// AsNumber returns the int64 payload of a Number value. The second
// return is false when the tag is not Number.
func (v Value) AsNumber() (int64, bool) {
	if v.typ == Number {
		return v.n, true
	}
	return 0, false
}

// AsBool returns the bool payload of a Bool value. The second return
// is false when the tag is not Bool.
func (v Value) AsBool() (bool, bool) {
	if v.typ == Bool {
		return v.b, true
	}
	return false, false
}

// Entry is one closed-table row describing a known option. All fields
// except Min/Max/Choices/Aliases/IsArray are required for every entry.
// See doc.go for the per-field semantics; this type is exported so
// downstream packages can reflect over Table (e.g. show-options once
// it lands in M2).
type Entry struct {
	Name    string
	Aliases []string
	Type    Type
	Scope   Scope
	Default Value
	Min     int64
	Max     int64
	Choices []string
	IsArray bool
	Help    string
}

// Table is the closed registry of known options. M1 ships exactly the
// four entries the walking skeleton reads; subsequent milestones add
// rows as their consumers land. Adding an option is one entry here
// plus one call site; resist scattering lookups.
var Table = []Entry{
	{
		Name:    "default-shell",
		Type:    String,
		Scope:   SessionScope,
		Default: NewString("/bin/sh"),
		Help:    "Shell run by new windows.",
	},
	{
		Name:    "default-terminal",
		Type:    String,
		Scope:   SessionScope,
		Default: NewString("xterm-256color"),
		Help:    "TERM value exported to child processes.",
	},
	{
		Name:    "status",
		Type:    Bool,
		Scope:   SessionScope,
		Default: NewBool(true),
		Help:    "Show the status line.",
	},
	{
		Name:    "status-position",
		Type:    Choice,
		Scope:   SessionScope,
		Default: NewChoice("bottom"),
		Choices: []string{"top", "bottom"},
		Help:    "Where to draw the status line.",
	},
}

// lookupEntry walks Table linearly by name or alias. The table is
// small (tens of entries once fully populated) and lookup is infrequent
// — a map would be more code for no measurable benefit.
func lookupEntry(name string) (*Entry, bool) {
	for i := range Table {
		e := &Table[i]
		if e.Name == name {
			return e, true
		}
		for _, a := range e.Aliases {
			if a == name {
				return e, true
			}
		}
	}
	return nil, false
}

// Options holds one scope's local option values and a pointer to its
// parent scope for Get-chain lookups. Methods are NOT safe for
// concurrent use; they are called only from the server's main
// goroutine (see doc.go, Concurrency).
type Options struct {
	scope  Scope
	parent *Options
	local  map[string]Value
}

// NewServerOptions constructs the root (server-scoped) Options. Its
// parent is nil; Get falls through to Table defaults on miss.
func NewServerOptions() *Options {
	return &Options{
		scope: ServerScope,
		local: make(map[string]Value),
	}
}

// NewScopedOptions constructs a child Options for scope with parent as
// its inheritance target. parent may be nil (typical for tests);
// callers that want the normal server -> session chain pass the server
// options through here.
func NewScopedOptions(scope Scope, parent *Options) *Options {
	return &Options{
		scope:  scope,
		parent: parent,
		local:  make(map[string]Value),
	}
}

// Get returns the most-specific value for name, walking the parent
// chain and finally falling back to the Table default. Unset user
// options (name starts with "@") read as NewString(""); other names
// not in the Table yield the zero Value. Callers that care about
// "is this set locally" use IsSetLocally instead of comparing
// against zero.
func (o *Options) Get(name string) Value {
	for cur := o; cur != nil; cur = cur.parent {
		if v, ok := cur.local[name]; ok {
			return v
		}
	}
	if IsUserOption(name) {
		return NewString("")
	}
	if e, ok := lookupEntry(name); ok {
		return e.Default
	}
	return Value{}
}

// GetString returns the string payload of Get(name). Panics if the
// resolved Value is not String or Choice — the Type comes from the
// closed Table so a mismatch is a programmer error caught at the first
// call (see doc.go).
func (o *Options) GetString(name string) string {
	v := o.Get(name)
	s, ok := v.AsString()
	if !ok {
		panic(fmt.Sprintf("options: GetString(%q): value is %v, not String or Choice", name, v.typ))
	}
	return s
}

// GetBool returns the bool payload of Get(name). Panics on tag
// mismatch for the same reason as GetString.
func (o *Options) GetBool(name string) bool {
	v := o.Get(name)
	b, ok := v.AsBool()
	if !ok {
		panic(fmt.Sprintf("options: GetBool(%q): value is %v, not Bool", name, v.typ))
	}
	return b
}

// GetNumber returns the int64 payload of Get(name). Panics on tag
// mismatch for the same reason as GetString.
func (o *Options) GetNumber(name string) int64 {
	v := o.Get(name)
	n, ok := v.AsNumber()
	if !ok {
		panic(fmt.Sprintf("options: GetNumber(%q): value is %v, not Number", name, v.typ))
	}
	return n
}

// Set writes v to this scope's local map after validating against the
// Table entry: the name must exist, v.Type must match the entry, this
// Options' scope bit must be present in entry.Scope, Choice values
// must match one of entry.Choices, and Number values must lie in
// [Min, Max] when Min/Max are non-zero. User options (name starts with
// "@") skip the Table lookup and scope check; they are always
// String-typed and accepted on any scope. Errors are structured
// *Error values wrapping the appropriate sentinel so callers can
// dispatch via errors.Is or pull detail via errors.As.
func (o *Options) Set(name string, v Value) error {
	if IsUserOption(name) {
		if v.typ != String {
			return &Error{
				Op:       "set",
				Name:     name,
				Sentinel: ErrTypeMismatch,
				Want:     String,
				Got:      v.typ,
			}
		}
		o.local[name] = v
		return nil
	}
	e, ok := lookupEntry(name)
	if !ok {
		return &Error{Op: "set", Name: name, Sentinel: ErrUnknownOption}
	}
	if v.typ != e.Type {
		return &Error{
			Op:       "set",
			Name:     name,
			Sentinel: ErrTypeMismatch,
			Want:     e.Type,
			Got:      v.typ,
		}
	}
	if e.Scope&o.scope == 0 {
		return &Error{
			Op:       "set",
			Name:     name,
			Sentinel: ErrScopeMismatch,
			Scope:    o.scope,
		}
	}
	if e.Type == Choice {
		found := false
		for _, c := range e.Choices {
			if c == v.s {
				found = true
				break
			}
		}
		if !found {
			return &Error{
				Op:       "set",
				Name:     name,
				Sentinel: ErrInvalidChoice,
				Choice:   v.s,
			}
		}
	}
	if e.Type == Number && !(e.Min == 0 && e.Max == 0) {
		if v.n < e.Min || v.n > e.Max {
			return &Error{
				Op:       "set",
				Name:     name,
				Sentinel: ErrOutOfRange,
				Number:   v.n,
				Min:      e.Min,
				Max:      e.Max,
			}
		}
	}
	o.local[name] = v
	return nil
}

// Unset removes name from this scope's local map. After Unset, Get on
// the same name walks the parent chain again. Returns ErrUnknownOption
// (wrapped) if name is not in the Table and not a user option;
// unsetting a name that was never set locally is not an error.
func (o *Options) Unset(name string) error {
	if IsUserOption(name) {
		delete(o.local, name)
		return nil
	}
	if _, ok := lookupEntry(name); !ok {
		return &Error{Op: "unset", Name: name, Sentinel: ErrUnknownOption}
	}
	delete(o.local, name)
	return nil
}

// IsSetLocally reports whether this scope has its own value for name,
// ignoring the parent chain and Table defaults.
func (o *Options) IsSetLocally(name string) bool {
	_, ok := o.local[name]
	return ok
}

// Sentinel errors. Callers use errors.Is to dispatch on the category;
// *Error carries the specifics for errors.As.
var (
	// ErrUnknownOption is returned by Set/Unset when name is not in
	// the closed Table and is not a user-defined `@` option (user
	// options land in M2).
	ErrUnknownOption = errors.New("options: unknown option")

	// ErrTypeMismatch is returned by Set when the passed Value's type
	// does not match the Table entry's Type.
	ErrTypeMismatch = errors.New("options: type mismatch")

	// ErrScopeMismatch is returned by Set when the Options instance's
	// scope is not in the Table entry's Scope bitmask.
	ErrScopeMismatch = errors.New("options: scope mismatch")

	// ErrInvalidChoice is returned by Set when a Choice Value's
	// payload is not one of the Table entry's Choices.
	ErrInvalidChoice = errors.New("options: invalid choice")

	// ErrOutOfRange is returned by Set when a Number Value falls
	// outside [Min, Max] (when the entry constrains it).
	ErrOutOfRange = errors.New("options: out of range")
)

// Error carries structured context for failures from this package.
// Sentinel is always one of the package sentinels, so errors.Is on the
// sentinel works regardless of whether the caller holds the concrete
// type. Name is the option involved; Op describes what was being
// attempted ("set", "unset"). The remaining fields are populated for
// the sentinels that need them:
//
//   - ErrTypeMismatch: Want (entry Type), Got (value Type)
//   - ErrScopeMismatch: Scope (the Options instance's scope)
//   - ErrInvalidChoice: Choice (the rejected payload)
//   - ErrOutOfRange: Number, Min, Max
//
// Err is an optional wrapped cause, kept for future lower-layer
// bubble-ups; M1 populates it nowhere.
type Error struct {
	Op       string
	Name     string
	Sentinel error

	Want, Got Type
	Scope     Scope
	Choice    string
	Number    int64
	Min, Max  int64

	Err error
}

// Error renders the failure as lowercase, no-trailing-punctuation
// text. Structured fields are appended in a fixed order so log-grep
// usage stays stable.
func (e *Error) Error() string {
	out := "options: " + e.Op
	if e.Sentinel != nil {
		tail := e.Sentinel.Error()
		const prefix = "options: "
		if len(tail) >= len(prefix) && tail[:len(prefix)] == prefix {
			tail = tail[len(prefix):]
		}
		out += ": " + tail
	}
	if e.Name != "" {
		out += fmt.Sprintf(" name=%q", e.Name)
	}
	switch e.Sentinel {
	case ErrTypeMismatch:
		out += fmt.Sprintf(" want=%v got=%v", e.Want, e.Got)
	case ErrScopeMismatch:
		out += fmt.Sprintf(" scope=%v", e.Scope)
	case ErrInvalidChoice:
		out += fmt.Sprintf(" choice=%q", e.Choice)
	case ErrOutOfRange:
		out += fmt.Sprintf(" number=%d min=%d max=%d", e.Number, e.Min, e.Max)
	}
	if e.Err != nil {
		out += ": " + e.Err.Error()
	}
	return out
}

// Unwrap returns the underlying cause (if any) so errors.Is/As
// traverses any future wrapped chain.
func (e *Error) Unwrap() error { return e.Err }

// Is matches the package sentinel even when Err carries an unrelated
// chain. Mirrors the session package pattern: the category is a
// property of Error, not of its cause.
func (e *Error) Is(target error) bool {
	return e.Sentinel != nil && e.Sentinel == target
}
