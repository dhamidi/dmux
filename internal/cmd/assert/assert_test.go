package assert_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmd/args"
	"github.com/dhamidi/dmux/internal/cmd/assert"
	"github.com/dhamidi/dmux/internal/options"
	"github.com/dhamidi/dmux/internal/proto"
)

// fakeClients implements cmd.ClientManager with only the surface the
// assert command exercises. Screen returns scripted bytes by ref;
// Spawn/Kill/Inject are present only to satisfy the interface and
// fail loudly if assert ever starts calling them.
type fakeClients struct {
	screens   map[string][]byte
	screenErr error
}

func newFakeClients() *fakeClients {
	return &fakeClients{screens: make(map[string][]byte)}
}

func (f *fakeClients) Spawn(profile string, cols, rows int) (string, error) {
	return "", errors.New("fakeClients.Spawn not implemented for assert tests")
}

func (f *fakeClients) Kill(ref string) error {
	return errors.New("fakeClients.Kill not implemented for assert tests")
}

func (f *fakeClients) Inject(ref string, bytes []byte) error {
	return errors.New("fakeClients.Inject not implemented for assert tests")
}

func (f *fakeClients) Screen(ref string) ([]byte, error) {
	if f.screenErr != nil {
		return nil, f.screenErr
	}
	b, ok := f.screens[ref]
	if !ok {
		return nil, fmt.Errorf("screen %s: %w", ref, cmd.ErrStaleClient)
	}
	return append([]byte(nil), b...), nil
}

type fakeItem struct {
	opts    *options.Options
	clients *fakeClients
}

func (*fakeItem) Context() context.Context           { return context.Background() }
func (*fakeItem) Shutdown(string)                    {}
func (*fakeItem) Client() cmd.Client                 { return nil }
func (*fakeItem) Sessions() cmd.SessionLookup        { return nil }
func (*fakeItem) SetAttachTarget(cmd.SessionRef)     {}
func (*fakeItem) SetDetach(proto.ExitReason, string) {}
func (i *fakeItem) Options() *options.Options        { return i.opts }
func (i *fakeItem) Clients() cmd.ClientManager       { return i.clients }
func (*fakeItem) CurrentSession() cmd.SessionRef     { return nil }
func (*fakeItem) SpawnWindow(cmd.SessionRef, string) (cmd.WindowRef, error) {
	return nil, nil
}
func (*fakeItem) AdvanceWindow(cmd.SessionRef, int) (cmd.WindowRef, error) {
	return nil, nil
}

func newFakeItem() *fakeItem {
	return &fakeItem{
		opts:    options.NewServerOptions(),
		clients: newFakeClients(),
	}
}

// bind associates name with ref in the fake so the assert command
// can resolve `@client/<name>` to ref and pull the scripted screen
// bytes from the fake's map.
func (i *fakeItem) bind(t *testing.T, name, ref string, screen []byte) {
	t.Helper()
	if err := i.opts.Set(assert.ClientOptionPrefix+name, options.NewString(ref)); err != nil {
		t.Fatalf("bind @client/%s: %v", name, err)
	}
	i.clients.screens[ref] = screen
}

func lookupAssert(t *testing.T) cmd.Command {
	t.Helper()
	c, ok := cmd.Lookup(assert.Name)
	if !ok {
		t.Fatalf("%q not registered", assert.Name)
	}
	return c
}

func TestScreenContainsMatches(t *testing.T) {
	c := lookupAssert(t)
	item := newFakeItem()
	item.bind(t, "foo", "cli-1", []byte("hello world"))

	res := c.Exec(item, []string{assert.Name, "screen", "contains", "foo", "hello"})
	if !res.OK() {
		t.Fatalf("assert screen contains returned %v, want Ok", res.Error())
	}
}

func TestScreenContainsMismatchWrapsErrAssertion(t *testing.T) {
	c := lookupAssert(t)
	item := newFakeItem()
	item.bind(t, "foo", "cli-1", []byte("hello world"))

	res := c.Exec(item, []string{assert.Name, "screen", "contains", "foo", "nope"})
	if res.OK() {
		t.Fatalf("assert with missing substring returned Ok, want Err")
	}
	if !errors.Is(res.Error(), assert.ErrAssertion) {
		t.Fatalf("error does not wrap ErrAssertion: %v", res.Error())
	}
}

func TestScreenContainsUnknownClientIsNotFound(t *testing.T) {
	c := lookupAssert(t)
	item := newFakeItem()

	res := c.Exec(item, []string{assert.Name, "screen", "contains", "ghost", "hi"})
	if res.OK() {
		t.Fatalf("assert against unknown client returned Ok, want Err")
	}
	if !errors.Is(res.Error(), cmd.ErrNotFound) {
		t.Fatalf("error does not wrap ErrNotFound: %v", res.Error())
	}
}

func TestScreenContainsStaleRefClearsOption(t *testing.T) {
	c := lookupAssert(t)
	item := newFakeItem()
	// Record the handle but leave the fake's screens map empty so
	// Screen returns a stale-ref error.
	if err := item.opts.Set(assert.ClientOptionPrefix+"foo", options.NewString("cli-1")); err != nil {
		t.Fatalf("setup option: %v", err)
	}

	res := c.Exec(item, []string{assert.Name, "screen", "contains", "foo", "hi"})
	if res.OK() {
		t.Fatalf("assert against stale ref returned Ok, want Err")
	}
	if !errors.Is(res.Error(), cmd.ErrStaleClient) {
		t.Fatalf("error does not wrap ErrStaleClient: %v", res.Error())
	}
	if got := item.opts.GetString(assert.ClientOptionPrefix + "foo"); got != "" {
		t.Fatalf("@client/foo = %q after stale-ref assert, want empty", got)
	}
}

func TestUnknownKindIsParseError(t *testing.T) {
	c := lookupAssert(t)
	item := newFakeItem()

	res := c.Exec(item, []string{assert.Name, "weather", "sunny"})
	if res.OK() {
		t.Fatalf("unknown kind returned Ok")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Name != "kind" {
		t.Fatalf("ParseError.Name = %q, want %q", perr.Name, "kind")
	}
	if perr.Value != "weather" {
		t.Fatalf("ParseError.Value = %q, want %q", perr.Value, "weather")
	}
}

func TestMissingKindIsParseError(t *testing.T) {
	c := lookupAssert(t)
	item := newFakeItem()

	res := c.Exec(item, []string{assert.Name})
	if res.OK() {
		t.Fatalf("missing kind returned Ok")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Name != "kind" {
		t.Fatalf("ParseError.Name = %q, want %q", perr.Name, "kind")
	}
}

func TestUnknownPredicateIsParseError(t *testing.T) {
	c := lookupAssert(t)
	item := newFakeItem()

	res := c.Exec(item, []string{assert.Name, "screen", "matches", "foo", "hi"})
	if res.OK() {
		t.Fatalf("unknown predicate returned Ok")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Name != "predicate" {
		t.Fatalf("ParseError.Name = %q, want %q", perr.Name, "predicate")
	}
	if perr.Value != "matches" {
		t.Fatalf("ParseError.Value = %q, want %q", perr.Value, "matches")
	}
}

func TestScreenContainsDecodesGoEscapes(t *testing.T) {
	c := lookupAssert(t)
	item := newFakeItem()
	item.bind(t, "foo", "cli-1", []byte("line one\nline two"))

	res := c.Exec(item, []string{assert.Name, "screen", "contains", "foo", `\n`})
	if !res.OK() {
		t.Fatalf("assert with \\n needle returned %v, want Ok", res.Error())
	}
}

func TestScreenContainsRejectsMalformedEscape(t *testing.T) {
	c := lookupAssert(t)
	item := newFakeItem()
	item.bind(t, "foo", "cli-1", []byte("hello"))

	res := c.Exec(item, []string{assert.Name, "screen", "contains", "foo", `\x`})
	if res.OK() {
		t.Fatalf("assert with malformed escape returned Ok, want Err")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Name != "text" {
		t.Fatalf("ParseError.Name = %q, want %q", perr.Name, "text")
	}
}

func TestScreenContainsRequiresClient(t *testing.T) {
	c := lookupAssert(t)
	item := newFakeItem()

	res := c.Exec(item, []string{assert.Name, "screen", "contains"})
	if res.OK() {
		t.Fatalf("assert without client returned Ok, want Err")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Name != "client" {
		t.Fatalf("ParseError.Name = %q, want %q", perr.Name, "client")
	}
}

func TestScreenContainsRequiresText(t *testing.T) {
	c := lookupAssert(t)
	item := newFakeItem()

	res := c.Exec(item, []string{assert.Name, "screen", "contains", "foo"})
	if res.OK() {
		t.Fatalf("assert without text returned Ok, want Err")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Name != "text" {
		t.Fatalf("ParseError.Name = %q, want %q", perr.Name, "text")
	}
}
