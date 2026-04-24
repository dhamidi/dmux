// Package args provides a thin layer over flag.FlagSet that adds
// typed positional parameters. A Set bundles a flag.FlagSet (for
// dashed options) with an ordered list of *flag.Flag for positionals.
// Positionals are declared exactly like flags — name, default, usage,
// and a flag.Value for storage — so introspection (help text,
// completion, diagnostics) works uniformly across both.
//
// Commands build a Set, register their flags and positionals, then
// call Parse(argv[1:]). After Parse, the typed destination pointers
// returned by the helper constructors hold the parsed values.
package args

import (
	"flag"
	"fmt"
	"io"
	"strconv"
)

// Set bundles a flag.FlagSet with an ordered list of positional
// parameters. The FlagSet handles dashed options; positionals consume
// the FlagSet.Args() tail in declaration order.
type Set struct {
	fs          *flag.FlagSet
	positionals []*flag.Flag
}

// New constructs a Set for a command called name. Errors surface as
// return values (flag.ContinueOnError) rather than os.Exit; usage
// output is silenced so callers format their own.
func New(name string) *Set {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return &Set{fs: fs}
}

// FlagSet exposes the underlying FlagSet. Callers register dashed
// options with its BoolVar / StringVar / IntVar / Var methods.
func (s *Set) FlagSet() *flag.FlagSet { return s.fs }

// Positional appends a pre-built *flag.Flag to the positional list.
// Useful for callers that already have a flag.Value implementation;
// most commands use the typed helpers below.
func (s *Set) Positional(f *flag.Flag) { s.positionals = append(s.positionals, f) }

// Positionals returns the declared positional flags in argv order.
// After Parse, each Flag's Value holds the parsed token (or its
// default if the tail was shorter than the positional list).
func (s *Set) Positionals() []*flag.Flag { return s.positionals }

// String declares a dashed string flag and returns a pointer to its
// destination.
func (s *Set) String(name, def, usage string) *string {
	return s.fs.String(name, def, usage)
}

// Bool declares a dashed bool flag and returns a pointer to its
// destination.
func (s *Set) Bool(name string, def bool, usage string) *bool {
	return s.fs.Bool(name, def, usage)
}

// Int declares a dashed int flag and returns a pointer to its
// destination.
func (s *Set) Int(name string, def int, usage string) *int {
	return s.fs.Int(name, def, usage)
}

// StringArg declares a string-typed positional. The returned pointer
// holds def before Parse and the parsed value after.
func (s *Set) StringArg(name, def, usage string) *string {
	p := new(string)
	*p = def
	s.positionals = append(s.positionals, &flag.Flag{
		Name:     name,
		Usage:    usage,
		Value:    (*stringValue)(p),
		DefValue: def,
	})
	return p
}

// BoolArg declares a bool-typed positional.
func (s *Set) BoolArg(name string, def bool, usage string) *bool {
	p := new(bool)
	*p = def
	s.positionals = append(s.positionals, &flag.Flag{
		Name:     name,
		Usage:    usage,
		Value:    (*boolValue)(p),
		DefValue: strconv.FormatBool(def),
	})
	return p
}

// IntArg declares an int-typed positional.
func (s *Set) IntArg(name string, def int, usage string) *int {
	p := new(int)
	*p = def
	s.positionals = append(s.positionals, &flag.Flag{
		Name:     name,
		Usage:    usage,
		Value:    (*intValue)(p),
		DefValue: strconv.Itoa(def),
	})
	return p
}

// Parse runs the FlagSet over argv, then distributes the remaining
// tokens across positionals in declaration order. Missing tokens
// leave positionals at their declared default; extra tokens are
// available via Rest. Parse errors are wrapped as *ParseError so
// callers can surface structured diagnostics.
func (s *Set) Parse(argv []string) error {
	if err := s.fs.Parse(argv); err != nil {
		return &ParseError{Phase: "flags", Err: err}
	}
	rest := s.fs.Args()
	for i, p := range s.positionals {
		if i >= len(rest) {
			break
		}
		if err := p.Value.Set(rest[i]); err != nil {
			return &ParseError{
				Phase: "positional",
				Name:  p.Name,
				Value: rest[i],
				Err:   err,
			}
		}
	}
	return nil
}

// Rest returns the argv tail after positionals were consumed. Empty
// when the tail was shorter than or equal in length to the positional
// list. Commands that take a trailing "everything else" argv (e.g.
// send-keys) read from here.
func (s *Set) Rest() []string {
	tail := s.fs.Args()
	if len(tail) <= len(s.positionals) {
		return nil
	}
	return tail[len(s.positionals):]
}

// ParseError carries structured context for argv parse failures.
// Phase is "flags" (error from flag.FlagSet.Parse) or "positional"
// (error from a positional flag.Value.Set). Name and Value are
// populated on positional errors.
type ParseError struct {
	Phase string
	Name  string
	Value string
	Err   error
}

// Error renders in the stdlib errors convention: lowercase, no
// trailing punctuation, structured fields appended in fixed order.
func (e *ParseError) Error() string {
	if e.Phase == "positional" {
		return fmt.Sprintf("args: positional %q: set %q: %v", e.Name, e.Value, e.Err)
	}
	return fmt.Sprintf("args: flag parse: %v", e.Err)
}

// Unwrap exposes the wrapped cause so errors.Is / errors.As traverses
// the chain (e.g. flag.ErrHelp is reachable from a flags-phase error).
func (e *ParseError) Unwrap() error { return e.Err }

type stringValue string

func (s *stringValue) Set(v string) error { *s = stringValue(v); return nil }
func (s *stringValue) String() string     { return string(*s) }

type boolValue bool

func (b *boolValue) Set(v string) error {
	p, err := strconv.ParseBool(v)
	if err != nil {
		return err
	}
	*b = boolValue(p)
	return nil
}
func (b *boolValue) String() string { return strconv.FormatBool(bool(*b)) }

type intValue int

func (i *intValue) Set(v string) error {
	p, err := strconv.Atoi(v)
	if err != nil {
		return err
	}
	*i = intValue(p)
	return nil
}
func (i *intValue) String() string { return strconv.Itoa(int(*i)) }
