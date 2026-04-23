package newsession_test

import (
	"context"
	"testing"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmd/newsession"
)

type fakeSessionRef struct{ id uint64 }

func (f fakeSessionRef) ID() uint64   { return f.id }
func (f fakeSessionRef) Name() string { return "fake" }

type fakeSessions struct{ createdID uint64 }

func (f *fakeSessions) Create(string) (cmd.SessionRef, error) {
	f.createdID++
	return fakeSessionRef{id: f.createdID}, nil
}
func (*fakeSessions) Find(string) (cmd.SessionRef, error) { return nil, cmd.ErrNotFound }
func (*fakeSessions) MostRecent() cmd.SessionRef          { return nil }
func (*fakeSessions) List() []cmd.SessionRef              { return nil }

type fakeItem struct {
	sessions *fakeSessions
	target   cmd.SessionRef
}

func (*fakeItem) Context() context.Context           { return context.Background() }
func (*fakeItem) Shutdown(string)                    {}
func (*fakeItem) Client() cmd.Client                 { return nil }
func (i *fakeItem) Sessions() cmd.SessionLookup      { return i.sessions }
func (i *fakeItem) SetAttachTarget(r cmd.SessionRef) { i.target = r }

func TestExecCreatesSessionAndSetsTarget(t *testing.T) {
	c, ok := cmd.Lookup(newsession.Name)
	if !ok {
		t.Fatalf("new-session not registered")
	}
	item := &fakeItem{sessions: &fakeSessions{}}
	res := c.Exec(item, []string{newsession.Name})
	if !res.OK() {
		t.Fatalf("Exec returned %v, want Ok", res.Error())
	}
	if item.target == nil {
		t.Fatalf("attach target not set after Ok Exec")
	}
	if item.sessions.createdID != 1 {
		t.Fatalf("Create not called exactly once: createdID=%d", item.sessions.createdID)
	}
}
