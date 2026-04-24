package detachclient_test

import (
	"context"
	"testing"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmd/detachclient"
	"github.com/dhamidi/dmux/internal/options"
	"github.com/dhamidi/dmux/internal/proto"
)

type fakeItem struct {
	detachSet     bool
	detachReason  proto.ExitReason
	detachMessage string
	detachCalls   int
}

func (*fakeItem) Context() context.Context       { return context.Background() }
func (*fakeItem) Shutdown(string)                {}
func (*fakeItem) Client() cmd.Client             { return nil }
func (*fakeItem) Sessions() cmd.SessionLookup    { return nil }
func (*fakeItem) SetAttachTarget(cmd.SessionRef) {}
func (*fakeItem) Options() *options.Options      { return nil }
func (*fakeItem) Clients() cmd.ClientManager     { return nil }
func (i *fakeItem) SetDetach(reason proto.ExitReason, message string) {
	i.detachSet = true
	i.detachReason = reason
	i.detachMessage = message
	i.detachCalls++
}

func TestExecRecordsDetach(t *testing.T) {
	c, ok := cmd.Lookup(detachclient.Name)
	if !ok {
		t.Fatalf("detach-client not registered")
	}
	item := &fakeItem{}
	res := c.Exec(item, []string{detachclient.Name})
	if !res.OK() {
		t.Fatalf("Exec returned %v, want Ok", res.Error())
	}
	if !item.detachSet {
		t.Fatalf("SetDetach was not called")
	}
	if item.detachCalls != 1 {
		t.Fatalf("SetDetach called %d times, want 1", item.detachCalls)
	}
	if item.detachReason != proto.ExitDetached {
		t.Fatalf("SetDetach reason = %v, want %v", item.detachReason, proto.ExitDetached)
	}
	if item.detachMessage != detachclient.Name {
		t.Fatalf("SetDetach message = %q, want %q", item.detachMessage, detachclient.Name)
	}
}

func TestExecIsIndependentPerItem(t *testing.T) {
	c, ok := cmd.Lookup(detachclient.Name)
	if !ok {
		t.Fatalf("detach-client not registered")
	}
	first := &fakeItem{}
	second := &fakeItem{}
	if res := c.Exec(first, []string{detachclient.Name}); !res.OK() {
		t.Fatalf("first Exec returned %v, want Ok", res.Error())
	}
	if res := c.Exec(second, []string{detachclient.Name}); !res.OK() {
		t.Fatalf("second Exec returned %v, want Ok", res.Error())
	}
	if first.detachCalls != 1 || second.detachCalls != 1 {
		t.Fatalf("detach calls first=%d second=%d, want 1 each", first.detachCalls, second.detachCalls)
	}
}
