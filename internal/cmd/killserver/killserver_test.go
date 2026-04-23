package killserver_test

import (
	"context"
	"testing"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmd/killserver"
)

type recordingItem struct{ msg string }

func (*recordingItem) Context() context.Context { return context.Background() }
func (*recordingItem) HasSession() bool         { return false }
func (i *recordingItem) Shutdown(m string)      { i.msg = m }

func TestExecCallsShutdown(t *testing.T) {
	c, ok := cmd.Lookup(killserver.Name)
	if !ok {
		t.Fatalf("kill-server not registered")
	}
	item := &recordingItem{}
	res := c.Exec(item, []string{killserver.Name})
	if !res.OK() {
		t.Fatalf("Exec returned %v, want Ok", res.Error())
	}
	if item.msg != killserver.Name {
		t.Fatalf("Shutdown called with %q, want %q", item.msg, killserver.Name)
	}
}
