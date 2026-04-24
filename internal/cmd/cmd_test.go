package cmd_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/options"
	"github.com/dhamidi/dmux/internal/proto"
)

type fakeItem struct{}

func (fakeItem) Context() context.Context             { return context.Background() }
func (fakeItem) Shutdown(string)                      {}
func (fakeItem) Client() cmd.Client                   { return nil }
func (fakeItem) Sessions() cmd.SessionLookup          { return nil }
func (fakeItem) SetAttachTarget(cmd.SessionRef)       {}
func (fakeItem) SetDetach(proto.ExitReason, string)   {}
func (fakeItem) Options() *options.Options            { return nil }
func (fakeItem) Clients() cmd.ClientManager           { return nil }

type fakeCmd struct {
	name   string
	called bool
}

func (f *fakeCmd) Name() string                            { return f.name }
func (f *fakeCmd) Exec(cmd.Item, []string) cmd.Result      { f.called = true; return cmd.Ok() }

func TestRegisterLookup(t *testing.T) {
	c := &fakeCmd{name: "cmd_test-echo"}
	cmd.Register(c)
	got, ok := cmd.Lookup("cmd_test-echo")
	if !ok {
		t.Fatalf("Lookup after Register returned ok=false")
	}
	if got.Name() != c.Name() {
		t.Fatalf("got name %q, want %q", got.Name(), c.Name())
	}
	if _, ok := cmd.Lookup("cmd_test-missing"); ok {
		t.Fatalf("Lookup of unregistered name returned ok=true")
	}
}

func TestResultOkErr(t *testing.T) {
	if !cmd.Ok().OK() {
		t.Fatalf("Ok().OK() == false")
	}
	if cmd.Ok().Error() != nil {
		t.Fatalf("Ok().Error() != nil")
	}
	sentinel := errors.New("x")
	r := cmd.Err(sentinel)
	if r.OK() {
		t.Fatalf("Err(...).OK() == true")
	}
	if !errors.Is(r.Error(), sentinel) {
		t.Fatalf("Err(sentinel).Error() does not wrap sentinel")
	}
}
