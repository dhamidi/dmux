package attachsession_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmd/attachsession"
)

type fakeItem struct{ has bool }

func (fakeItem) Context() context.Context { return context.Background() }
func (fakeItem) Shutdown(string)          {}
func (f fakeItem) HasSession() bool       { return f.has }

func TestExec(t *testing.T) {
	c, ok := cmd.Lookup(attachsession.Name)
	if !ok {
		t.Fatalf("attach-session not registered")
	}
	if res := c.Exec(fakeItem{has: true}, []string{attachsession.Name}); !res.OK() {
		t.Fatalf("has=true Exec returned %v, want Ok", res.Error())
	}
	res := c.Exec(fakeItem{has: false}, []string{attachsession.Name})
	if res.OK() {
		t.Fatalf("has=false Exec returned Ok, want Err")
	}
	if !errors.Is(res.Error(), cmd.ErrNotFound) {
		t.Fatalf("has=false error does not wrap ErrNotFound: %v", res.Error())
	}
}
