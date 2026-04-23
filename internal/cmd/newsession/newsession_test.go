package newsession_test

import (
	"context"
	"testing"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmd/newsession"
)

type fakeItem struct{}

func (fakeItem) Context() context.Context { return context.Background() }
func (fakeItem) Shutdown(string)          {}
func (fakeItem) HasSession() bool         { return false }

func TestExecReturnsOk(t *testing.T) {
	c, ok := cmd.Lookup(newsession.Name)
	if !ok {
		t.Fatalf("new-session not registered")
	}
	res := c.Exec(fakeItem{}, []string{newsession.Name})
	if !res.OK() {
		t.Fatalf("Exec returned %v, want Ok", res.Error())
	}
}
