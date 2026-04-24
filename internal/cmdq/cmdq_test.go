package cmdq_test

import (
	"context"
	"testing"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmdq"
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

type recordingCmd struct {
	name  string
	calls *[][]string
	res   cmd.Result
}

func (r *recordingCmd) Name() string { return r.name }
func (r *recordingCmd) Exec(_ cmd.Item, argv []string) cmd.Result {
	*r.calls = append(*r.calls, argv)
	return r.res
}

func TestListAppendDrainOrder(t *testing.T) {
	var calls [][]string
	a := &recordingCmd{name: "a", calls: &calls, res: cmd.Ok()}
	b := &recordingCmd{name: "b", calls: &calls, res: cmd.Err(cmd.ErrNotFound)}
	var l cmdq.List
	l.Append(cmdq.Item{Cmd: a, Argv: []string{"a"}, CmdItem: fakeItem{}})
	l.Append(cmdq.Item{Cmd: b, Argv: []string{"b", "x"}, CmdItem: fakeItem{}})
	results := l.Drain()
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if !results[0].OK() || results[1].OK() {
		t.Fatalf("result OK pattern wrong: %v / %v", results[0].OK(), results[1].OK())
	}
	if len(calls) != 2 || calls[0][0] != "a" || calls[1][0] != "b" {
		t.Fatalf("exec order wrong: %v", calls)
	}
	if l.Len() != 0 {
		t.Fatalf("list not drained: len=%d", l.Len())
	}
}
