package newwindow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmd/newwindow"
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

type spawnCall struct {
	sess cmd.SessionRef
	name string
}

type fakeItem struct {
	session    cmd.SessionRef
	spawnCalls []spawnCall
	spawnErr   error
	spawnRef   cmd.WindowRef
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
func (i *fakeItem) SpawnWindow(sess cmd.SessionRef, name string) (cmd.WindowRef, error) {
	i.spawnCalls = append(i.spawnCalls, spawnCall{sess: sess, name: name})
	if i.spawnErr != nil {
		return nil, i.spawnErr
	}
	return i.spawnRef, nil
}
func (*fakeItem) AdvanceWindow(cmd.SessionRef, int) (cmd.WindowRef, error) {
	return nil, nil
}

func TestExecAppendsWindowInCurrentSession(t *testing.T) {
	c, ok := cmd.Lookup(newwindow.Name)
	if !ok {
		t.Fatalf("new-window not registered")
	}
	sess := fakeSessionRef{id: 7}
	win := fakeWindowRef{index: 1, name: "bash"}
	item := &fakeItem{session: sess, spawnRef: win}
	res := c.Exec(item, []string{newwindow.Name})
	if !res.OK() {
		t.Fatalf("Exec returned %v, want Ok", res.Error())
	}
	if len(item.spawnCalls) != 1 {
		t.Fatalf("SpawnWindow called %d times, want 1", len(item.spawnCalls))
	}
	call := item.spawnCalls[0]
	if call.sess != sess {
		t.Fatalf("SpawnWindow session = %v, want %v", call.sess, sess)
	}
	if call.name != "" {
		t.Fatalf("SpawnWindow name = %q, want empty (server-default)", call.name)
	}
}

func TestExecWithoutCurrentSessionIsNotFound(t *testing.T) {
	c, ok := cmd.Lookup(newwindow.Name)
	if !ok {
		t.Fatalf("new-window not registered")
	}
	item := &fakeItem{}
	res := c.Exec(item, []string{newwindow.Name})
	if res.OK() {
		t.Fatalf("Exec with no current session returned Ok, want Err")
	}
	if !errors.Is(res.Error(), cmd.ErrNotFound) {
		t.Fatalf("error does not wrap ErrNotFound: %v", res.Error())
	}
	if len(item.spawnCalls) != 0 {
		t.Fatalf("SpawnWindow called without a session: %v", item.spawnCalls)
	}
}

func TestExecPropagatesSpawnFailure(t *testing.T) {
	c, ok := cmd.Lookup(newwindow.Name)
	if !ok {
		t.Fatalf("new-window not registered")
	}
	boom := errors.New("server: simulated pane open failure")
	item := &fakeItem{session: fakeSessionRef{id: 1}, spawnErr: boom}
	res := c.Exec(item, []string{newwindow.Name})
	if res.OK() {
		t.Fatalf("Exec with spawn failure returned Ok, want Err")
	}
	if !errors.Is(res.Error(), boom) {
		t.Fatalf("error does not wrap boom: %v", res.Error())
	}
}
