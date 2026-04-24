package attachsession_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmd/attachsession"
	"github.com/dhamidi/dmux/internal/options"
)

type fakeSessionRef struct{}

func (fakeSessionRef) ID() uint64   { return 1 }
func (fakeSessionRef) Name() string { return "fake" }

type fakeSessions struct{ ref cmd.SessionRef }

func (f fakeSessions) Create(string) (cmd.SessionRef, error) { return nil, errors.New("not used") }
func (f fakeSessions) Find(string) (cmd.SessionRef, error)   { return nil, cmd.ErrNotFound }
func (f fakeSessions) MostRecent() cmd.SessionRef            { return f.ref }
func (f fakeSessions) List() []cmd.SessionRef                { return nil }

type fakeItem struct {
	sessions fakeSessions
	target   cmd.SessionRef
}

func (*fakeItem) Context() context.Context            { return context.Background() }
func (*fakeItem) Shutdown(string)                     {}
func (*fakeItem) Client() cmd.Client                  { return nil }
func (i *fakeItem) Sessions() cmd.SessionLookup      { return i.sessions }
func (i *fakeItem) SetAttachTarget(r cmd.SessionRef) { i.target = r }
func (*fakeItem) Options() *options.Options          { return nil }
func (*fakeItem) Clients() cmd.ClientManager         { return nil }

func TestExec(t *testing.T) {
	c, ok := cmd.Lookup(attachsession.Name)
	if !ok {
		t.Fatalf("attach-session not registered")
	}
	ref := fakeSessionRef{}
	present := &fakeItem{sessions: fakeSessions{ref: ref}}
	if res := c.Exec(present, []string{attachsession.Name}); !res.OK() {
		t.Fatalf("MostRecent!=nil Exec returned %v, want Ok", res.Error())
	}
	if present.target != ref {
		t.Fatalf("attach target = %v, want %v", present.target, ref)
	}

	absent := &fakeItem{sessions: fakeSessions{ref: nil}}
	res := c.Exec(absent, []string{attachsession.Name})
	if res.OK() {
		t.Fatalf("MostRecent=nil Exec returned Ok, want Err")
	}
	if !errors.Is(res.Error(), cmd.ErrNotFound) {
		t.Fatalf("MostRecent=nil error does not wrap ErrNotFound: %v", res.Error())
	}
	if absent.target != nil {
		t.Fatalf("attach target set on failed path: %v", absent.target)
	}
}
