package args_test

import (
	"errors"
	"flag"
	"testing"

	"github.com/dhamidi/dmux/internal/cmd/args"
)

func TestParseFlagsAndPositionals(t *testing.T) {
	s := args.New("new-session")
	name := s.String("s", "", "session name")
	detached := s.Bool("d", false, "detached")
	cmd := s.StringArg("command", "/bin/sh", "shell to run")

	if err := s.Parse([]string{"-s", "work", "-d", "/bin/zsh"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if *name != "work" {
		t.Fatalf("-s: got %q, want %q", *name, "work")
	}
	if !*detached {
		t.Fatalf("-d not set")
	}
	if *cmd != "/bin/zsh" {
		t.Fatalf("command: got %q, want %q", *cmd, "/bin/zsh")
	}
}

func TestPositionalDefaultsApply(t *testing.T) {
	s := args.New("new-session")
	cmd := s.StringArg("command", "/bin/sh", "shell to run")

	if err := s.Parse(nil); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if *cmd != "/bin/sh" {
		t.Fatalf("default: got %q, want %q", *cmd, "/bin/sh")
	}
}

func TestRestReturnsTrailingArgv(t *testing.T) {
	s := args.New("send-keys")
	target := s.String("t", "", "target")
	first := s.StringArg("subcommand", "", "subcommand")

	if err := s.Parse([]string{"-t", "work", "literal", "echo", "hi"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if *target != "work" {
		t.Fatalf("-t: got %q, want %q", *target, "work")
	}
	if *first != "literal" {
		t.Fatalf("first positional: got %q, want %q", *first, "literal")
	}
	rest := s.Rest()
	if len(rest) != 2 || rest[0] != "echo" || rest[1] != "hi" {
		t.Fatalf("Rest: got %v, want [echo hi]", rest)
	}
}

func TestTypedPositionals(t *testing.T) {
	s := args.New("resize")
	cols := s.IntArg("cols", 80, "columns")
	strict := s.BoolArg("strict", false, "strict mode")

	if err := s.Parse([]string{"120", "true"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if *cols != 120 {
		t.Fatalf("cols: got %d, want 120", *cols)
	}
	if !*strict {
		t.Fatalf("strict: got false, want true")
	}
}

func TestPositionalParseErrorIsStructured(t *testing.T) {
	s := args.New("resize")
	_ = s.IntArg("cols", 0, "columns")

	err := s.Parse([]string{"not-a-number"})
	if err == nil {
		t.Fatalf("Parse returned nil, want error")
	}
	var perr *args.ParseError
	if !errors.As(err, &perr) {
		t.Fatalf("errors.As(*args.ParseError) failed: %v", err)
	}
	if perr.Phase != "positional" || perr.Name != "cols" || perr.Value != "not-a-number" {
		t.Fatalf("ParseError fields wrong: %+v", perr)
	}
}

func TestFlagErrorWrapsErrHelp(t *testing.T) {
	s := args.New("cmd")
	_ = s.String("s", "", "session")

	err := s.Parse([]string{"-h"})
	if err == nil {
		t.Fatalf("Parse with -h returned nil, want flag.ErrHelp wrapped")
	}
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("errors.Is(flag.ErrHelp) failed: %v", err)
	}
}

func TestPositionalsIntrospection(t *testing.T) {
	s := args.New("cmd")
	_ = s.StringArg("handle", "", "client handle")
	_ = s.IntArg("cols", 80, "columns")

	got := s.Positionals()
	if len(got) != 2 {
		t.Fatalf("len(Positionals)=%d, want 2", len(got))
	}
	if got[0].Name != "handle" || got[1].Name != "cols" {
		t.Fatalf("declaration order: %q, %q", got[0].Name, got[1].Name)
	}
	if got[1].DefValue != "80" {
		t.Fatalf("cols DefValue: got %q, want %q", got[1].DefValue, "80")
	}
}
