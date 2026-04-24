package recorder_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmd/args"
	"github.com/dhamidi/dmux/internal/cmd/recorder"
	"github.com/dhamidi/dmux/internal/options"
	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/record"
)

// fakeItem satisfies cmd.Item by returning nil / no-ops from every
// method: recorder set-level touches no capability on Item — it
// flips state through the package-level record.SetLevel.
type fakeItem struct{}

func (*fakeItem) Context() context.Context             { return context.Background() }
func (*fakeItem) Shutdown(string)                      {}
func (*fakeItem) Client() cmd.Client                   { return nil }
func (*fakeItem) Sessions() cmd.SessionLookup          { return nil }
func (*fakeItem) SetAttachTarget(cmd.SessionRef)       {}
func (*fakeItem) SetDetach(proto.ExitReason, string)   {}
func (*fakeItem) Options() *options.Options            { return nil }
func (*fakeItem) Clients() cmd.ClientManager           { return nil }

func lookupRecorder(t *testing.T) cmd.Command {
	t.Helper()
	c, ok := cmd.Lookup(recorder.Name)
	if !ok {
		t.Fatalf("%q not registered", recorder.Name)
	}
	return c
}

// withOpenRecorder brackets a test with record.Open / record.Close
// so SetLevel takes effect (SetLevel is a no-op when no recorder is
// open). The logger discards output; level checks are through
// CurrentLevel, not by observing log lines.
func withOpenRecorder(t *testing.T) {
	t.Helper()
	if err := record.Open(record.Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}); err != nil {
		t.Fatalf("record.Open: %v", err)
	}
	t.Cleanup(func() { _ = record.Close() })
}

func TestSetLevelDebugPromotesCurrentLevel(t *testing.T) {
	withOpenRecorder(t)
	if got := record.CurrentLevel(); got != record.LevelNormal {
		t.Fatalf("pre-exec CurrentLevel = %v, want LevelNormal", got)
	}

	c := lookupRecorder(t)
	res := c.Exec(&fakeItem{}, []string{recorder.Name, "set-level", "debug"})
	if !res.OK() {
		t.Fatalf("set-level debug returned %v, want Ok", res.Error())
	}
	if got := record.CurrentLevel(); got != record.LevelDebug {
		t.Fatalf("post-exec CurrentLevel = %v, want LevelDebug", got)
	}
}

func TestSetLevelNormalKeepsCurrentLevel(t *testing.T) {
	withOpenRecorder(t)
	record.SetLevel(record.LevelDebug)

	c := lookupRecorder(t)
	res := c.Exec(&fakeItem{}, []string{recorder.Name, "set-level", "normal"})
	if !res.OK() {
		t.Fatalf("set-level normal returned %v, want Ok", res.Error())
	}
	if got := record.CurrentLevel(); got != record.LevelNormal {
		t.Fatalf("post-exec CurrentLevel = %v, want LevelNormal", got)
	}
}

func TestSetLevelRejectsUnknownLevelName(t *testing.T) {
	c := lookupRecorder(t)
	res := c.Exec(&fakeItem{}, []string{recorder.Name, "set-level", "loud"})
	if res.OK() {
		t.Fatalf("set-level loud returned Ok, want Err")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Phase != "positional" {
		t.Fatalf("ParseError.Phase = %q, want %q", perr.Phase, "positional")
	}
	if perr.Name != "level" {
		t.Fatalf("ParseError.Name = %q, want %q", perr.Name, "level")
	}
	if perr.Value != "loud" {
		t.Fatalf("ParseError.Value = %q, want %q", perr.Value, "loud")
	}
}

func TestSetLevelRejectsMissingPositional(t *testing.T) {
	c := lookupRecorder(t)
	res := c.Exec(&fakeItem{}, []string{recorder.Name, "set-level"})
	if res.OK() {
		t.Fatalf("set-level without argument returned Ok, want Err")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Phase != "positional" {
		t.Fatalf("ParseError.Phase = %q, want %q", perr.Phase, "positional")
	}
	if perr.Name != "level" {
		t.Fatalf("ParseError.Name = %q, want %q", perr.Name, "level")
	}
}

func TestUnknownSubcommandIsParseError(t *testing.T) {
	c := lookupRecorder(t)
	res := c.Exec(&fakeItem{}, []string{recorder.Name, "dance", "debug"})
	if res.OK() {
		t.Fatalf("unknown subcommand returned Ok, want Err")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Phase != "positional" {
		t.Fatalf("ParseError.Phase = %q, want %q", perr.Phase, "positional")
	}
	if perr.Name != "subcommand" {
		t.Fatalf("ParseError.Name = %q, want %q", perr.Name, "subcommand")
	}
	if perr.Value != "dance" {
		t.Fatalf("ParseError.Value = %q, want %q", perr.Value, "dance")
	}
}

func TestMissingSubcommandIsParseError(t *testing.T) {
	c := lookupRecorder(t)
	res := c.Exec(&fakeItem{}, []string{recorder.Name})
	if res.OK() {
		t.Fatalf("missing subcommand returned Ok, want Err")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Name != "subcommand" {
		t.Fatalf("ParseError.Name = %q, want %q", perr.Name, "subcommand")
	}
}
