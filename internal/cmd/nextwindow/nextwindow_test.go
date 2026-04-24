package nextwindow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmd/nextwindow"
	"github.com/dhamidi/dmux/internal/options"
	"github.com/dhamidi/dmux/internal/proto"
)

type fakeSessionRef struct{ id uint64 }

func (f fakeSessionRef) ID() uint64   { return f.id }
func (f fakeSessionRef) Name() string { return "fake" }

type fakeWindowRef struct {
	index int
	name  string
}

func (f fakeWindowRef) Index() int   { return f.index }
func (f fakeWindowRef) Name() string { return f.name }

type advanceCall struct {
	sess  cmd.SessionRef
	delta int
}

type fakeItem struct {
	session      cmd.SessionRef
	advanceCalls []advanceCall
	advanceErr   error
	advanceRef   cmd.WindowRef
}

func (*fakeItem) Context() context.Context           { return context.Background() }
func (*fakeItem) Shutdown(string)                    {}
func (*fakeItem) Client() cmd.Client                 { return nil }
func (*fakeItem) Sessions() cmd.SessionLookup        { return nil }
func (*fakeItem) SetAttachTarget(cmd.SessionRef)     {}
func (*fakeItem) SetDetach(proto.ExitReason, string) {}
func (*fakeItem) Options() *options.Options          { return nil }
func (*fakeItem) Clients() cmd.ClientManager         { return nil }
func (i *fakeItem) CurrentSession() cmd.SessionRef   { return i.session }
func (*fakeItem) SpawnWindow(cmd.SessionRef, string) (cmd.WindowRef, error) {
	return nil, nil
}
func (i *fakeItem) AdvanceWindow(sess cmd.SessionRef, delta int) (cmd.WindowRef, error) {
	i.advanceCalls = append(i.advanceCalls, advanceCall{sess: sess, delta: delta})
	if i.advanceErr != nil {
		return nil, i.advanceErr
	}
	return i.advanceRef, nil
}

func TestExecAdvancesByPlusOne(t *testing.T) {
	c, ok := cmd.Lookup(nextwindow.Name)
	if !ok {
		t.Fatalf("next-window not registered")
	}
	sess := fakeSessionRef{id: 3}
	win := fakeWindowRef{index: 1, name: "vim"}
	item := &fakeItem{session: sess, advanceRef: win}
	res := c.Exec(item, []string{nextwindow.Name})
	if !res.OK() {
		t.Fatalf("Exec returned %v, want Ok", res.Error())
	}
	if len(item.advanceCalls) != 1 {
		t.Fatalf("AdvanceWindow called %d times, want 1", len(item.advanceCalls))
	}
	call := item.advanceCalls[0]
	if call.sess != sess {
		t.Fatalf("AdvanceWindow session = %v, want %v", call.sess, sess)
	}
	if call.delta != 1 {
		t.Fatalf("AdvanceWindow delta = %d, want 1", call.delta)
	}
}

func TestExecWithoutCurrentSessionIsNotFound(t *testing.T) {
	c, ok := cmd.Lookup(nextwindow.Name)
	if !ok {
		t.Fatalf("next-window not registered")
	}
	item := &fakeItem{}
	res := c.Exec(item, []string{nextwindow.Name})
	if res.OK() {
		t.Fatalf("Exec with no current session returned Ok, want Err")
	}
	if !errors.Is(res.Error(), cmd.ErrNotFound) {
		t.Fatalf("error does not wrap ErrNotFound: %v", res.Error())
	}
	if len(item.advanceCalls) != 0 {
		t.Fatalf("AdvanceWindow called without a session: %v", item.advanceCalls)
	}
}

func TestExecPropagatesAdvanceFailure(t *testing.T) {
	c, ok := cmd.Lookup(nextwindow.Name)
	if !ok {
		t.Fatalf("next-window not registered")
	}
	boom := errors.New("server: simulated advance failure")
	item := &fakeItem{session: fakeSessionRef{id: 1}, advanceErr: boom}
	res := c.Exec(item, []string{nextwindow.Name})
	if res.OK() {
		t.Fatalf("Exec with advance failure returned Ok, want Err")
	}
	if !errors.Is(res.Error(), boom) {
		t.Fatalf("error does not wrap boom: %v", res.Error())
	}
}
