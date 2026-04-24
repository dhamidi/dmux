package client_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmd/args"
	"github.com/dhamidi/dmux/internal/cmd/client"
	"github.com/dhamidi/dmux/internal/options"
	"github.com/dhamidi/dmux/internal/proto"
)

type fakeClients struct {
	next      int
	live      map[string]bool
	spawnErr  error
	killErr   error
	injectErr error
	spawned   []spawnCall
	killed    []string
	injected  []injectCall
}

type spawnCall struct {
	profile string
	cols    int
	rows    int
}

type injectCall struct {
	ref   string
	bytes []byte
}

func newFakeClients() *fakeClients {
	return &fakeClients{live: make(map[string]bool)}
}

func (f *fakeClients) Spawn(profile string, cols, rows int) (string, error) {
	if f.spawnErr != nil {
		return "", f.spawnErr
	}
	f.next++
	ref := fmt.Sprintf("cli-%d", f.next)
	f.live[ref] = true
	f.spawned = append(f.spawned, spawnCall{profile, cols, rows})
	return ref, nil
}

func (f *fakeClients) Kill(ref string) error {
	f.killed = append(f.killed, ref)
	if f.killErr != nil {
		return f.killErr
	}
	if !f.live[ref] {
		return fmt.Errorf("kill %s: %w", ref, cmd.ErrStaleClient)
	}
	delete(f.live, ref)
	return nil
}

func (f *fakeClients) Inject(ref string, bytes []byte) error {
	f.injected = append(f.injected, injectCall{ref: ref, bytes: bytes})
	if f.injectErr != nil {
		return f.injectErr
	}
	if !f.live[ref] {
		return fmt.Errorf("inject %s: %w", ref, cmd.ErrStaleClient)
	}
	return nil
}

type fakeItem struct {
	opts    *options.Options
	clients *fakeClients
}

func (*fakeItem) Context() context.Context             { return context.Background() }
func (*fakeItem) Shutdown(string)                      {}
func (*fakeItem) Client() cmd.Client                   { return nil }
func (*fakeItem) Sessions() cmd.SessionLookup          { return nil }
func (*fakeItem) SetAttachTarget(cmd.SessionRef)       {}
func (*fakeItem) SetDetach(proto.ExitReason, string)   {}
func (i *fakeItem) Options() *options.Options          { return i.opts }
func (i *fakeItem) Clients() cmd.ClientManager         { return i.clients }
func (*fakeItem) CurrentSession() cmd.SessionRef       { return nil }
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

func lookupClient(t *testing.T) cmd.Command {
	t.Helper()
	c, ok := cmd.Lookup(client.Name)
	if !ok {
		t.Fatalf("%q not registered", client.Name)
	}
	return c
}

func TestSpawnStoresHandleInUserOption(t *testing.T) {
	c := lookupClient(t)
	item := newFakeItem()
	res := c.Exec(item, []string{client.Name, "spawn", "shell"})
	if !res.OK() {
		t.Fatalf("spawn returned %v, want Ok", res.Error())
	}
	got := item.opts.GetString(client.OptionPrefix + "shell")
	if got == "" {
		t.Fatalf("@client/shell is empty after spawn")
	}
	if !item.clients.live[got] {
		t.Fatalf("Spawn handle %q not live in fake clients", got)
	}
	if len(item.clients.spawned) != 1 {
		t.Fatalf("Spawn called %d times, want 1", len(item.clients.spawned))
	}
}

func TestSpawnPropagatesFlags(t *testing.T) {
	c := lookupClient(t)
	item := newFakeItem()
	res := c.Exec(item, []string{client.Name, "spawn", "-F", "ai", "-x", "120", "-y", "40", "agent"})
	if !res.OK() {
		t.Fatalf("spawn returned %v, want Ok", res.Error())
	}
	if len(item.clients.spawned) != 1 {
		t.Fatalf("Spawn not called exactly once: %v", item.clients.spawned)
	}
	got := item.clients.spawned[0]
	if got.profile != "ai" || got.cols != 120 || got.rows != 40 {
		t.Fatalf("Spawn args = %+v, want {profile:ai cols:120 rows:40}", got)
	}
}

func TestSpawnRejectsMissingHandle(t *testing.T) {
	c := lookupClient(t)
	item := newFakeItem()
	res := c.Exec(item, []string{client.Name, "spawn"})
	if res.OK() {
		t.Fatalf("spawn without handle returned Ok, want Err")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Name != "name" {
		t.Fatalf("ParseError.Name = %q, want %q", perr.Name, "name")
	}
	if len(item.clients.spawned) != 0 {
		t.Fatalf("Spawn called despite parse error: %v", item.clients.spawned)
	}
}

func TestKillUnsetsOptionAndTearsDown(t *testing.T) {
	c := lookupClient(t)
	item := newFakeItem()
	if res := c.Exec(item, []string{client.Name, "spawn", "shell"}); !res.OK() {
		t.Fatalf("setup spawn: %v", res.Error())
	}
	ref := item.opts.GetString(client.OptionPrefix + "shell")
	res := c.Exec(item, []string{client.Name, "kill", "shell"})
	if !res.OK() {
		t.Fatalf("kill returned %v, want Ok", res.Error())
	}
	if got := item.opts.GetString(client.OptionPrefix + "shell"); got != "" {
		t.Fatalf("@client/shell = %q after kill, want empty", got)
	}
	if item.clients.live[ref] {
		t.Fatalf("Kill did not tear down %q", ref)
	}
	if len(item.clients.killed) != 1 || item.clients.killed[0] != ref {
		t.Fatalf("Kill call log = %v, want [%q]", item.clients.killed, ref)
	}
}

func TestKillOnMissingOptionIsNoop(t *testing.T) {
	c := lookupClient(t)
	item := newFakeItem()
	res := c.Exec(item, []string{client.Name, "kill", "ghost"})
	if !res.OK() {
		t.Fatalf("kill of unknown name returned %v, want Ok", res.Error())
	}
	if len(item.clients.killed) != 0 {
		t.Fatalf("Kill called with no stored ref: %v", item.clients.killed)
	}
}

func TestKillToleratesStaleReference(t *testing.T) {
	c := lookupClient(t)
	item := newFakeItem()
	if res := c.Exec(item, []string{client.Name, "spawn", "shell"}); !res.OK() {
		t.Fatalf("setup spawn: %v", res.Error())
	}
	ref := item.opts.GetString(client.OptionPrefix + "shell")
	// Simulate the client already being gone — option still points at
	// it, but fakeClients no longer tracks it as live.
	delete(item.clients.live, ref)

	res := c.Exec(item, []string{client.Name, "kill", "shell"})
	if !res.OK() {
		t.Fatalf("kill of stale ref returned %v, want Ok", res.Error())
	}
	if got := item.opts.GetString(client.OptionPrefix + "shell"); got != "" {
		t.Fatalf("@client/shell = %q after stale-ref kill, want empty", got)
	}
}

func TestKillPropagatesNonStaleError(t *testing.T) {
	c := lookupClient(t)
	item := newFakeItem()
	if res := c.Exec(item, []string{client.Name, "spawn", "shell"}); !res.OK() {
		t.Fatalf("setup spawn: %v", res.Error())
	}
	boom := errors.New("clients: simulated teardown failure")
	item.clients.killErr = boom

	res := c.Exec(item, []string{client.Name, "kill", "shell"})
	if res.OK() {
		t.Fatalf("kill with underlying teardown error returned Ok")
	}
	if !errors.Is(res.Error(), boom) {
		t.Fatalf("kill error does not wrap boom: %v", res.Error())
	}
	// Option was unset before the Kill call — matches the design.
	if got := item.opts.GetString(client.OptionPrefix + "shell"); got != "" {
		t.Fatalf("@client/shell = %q after kill failure, want empty", got)
	}
}

func TestUnknownSubcommandIsParseError(t *testing.T) {
	c := lookupClient(t)
	item := newFakeItem()
	res := c.Exec(item, []string{client.Name, "dance", "shell"})
	if res.OK() {
		t.Fatalf("unknown subcommand returned Ok")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Value != "dance" {
		t.Fatalf("ParseError.Value = %q, want %q", perr.Value, "dance")
	}
}

func TestSpawnErrorFromManagerPropagates(t *testing.T) {
	c := lookupClient(t)
	item := newFakeItem()
	boom := errors.New("clients: simulated spawn failure")
	item.clients.spawnErr = boom

	res := c.Exec(item, []string{client.Name, "spawn", "shell"})
	if res.OK() {
		t.Fatalf("spawn with underlying failure returned Ok")
	}
	if !errors.Is(res.Error(), boom) {
		t.Fatalf("spawn error does not wrap boom: %v", res.Error())
	}
	if got := item.opts.GetString(client.OptionPrefix + "shell"); got != "" {
		t.Fatalf("@client/shell = %q after spawn failure, want empty", got)
	}
}

func TestAtDeliversBytesToLiveClient(t *testing.T) {
	c := lookupClient(t)
	item := newFakeItem()
	if res := c.Exec(item, []string{client.Name, "spawn", "shell"}); !res.OK() {
		t.Fatalf("setup spawn: %v", res.Error())
	}
	ref := item.opts.GetString(client.OptionPrefix + "shell")

	res := c.Exec(item, []string{client.Name, "at", "shell", `echo hi\n`})
	if !res.OK() {
		t.Fatalf("at returned %v, want Ok", res.Error())
	}
	if len(item.clients.injected) != 1 {
		t.Fatalf("Inject called %d times, want 1: %v", len(item.clients.injected), item.clients.injected)
	}
	got := item.clients.injected[0]
	if got.ref != ref {
		t.Fatalf("Inject ref = %q, want %q", got.ref, ref)
	}
	if string(got.bytes) != "echo hi\n" {
		t.Fatalf("Inject bytes = %q, want %q", got.bytes, "echo hi\n")
	}
}

func TestAtParsesGoEscapes(t *testing.T) {
	c := lookupClient(t)

	// Hex escape: "\x02d" → 0x02 'd' (Ctrl-B d).
	item := newFakeItem()
	if res := c.Exec(item, []string{client.Name, "spawn", "shell"}); !res.OK() {
		t.Fatalf("setup spawn: %v", res.Error())
	}
	res := c.Exec(item, []string{client.Name, "at", "shell", `\x02d`})
	if !res.OK() {
		t.Fatalf("at with hex escape returned %v, want Ok", res.Error())
	}
	if len(item.clients.injected) != 1 {
		t.Fatalf("Inject called %d times, want 1", len(item.clients.injected))
	}
	got := item.clients.injected[0].bytes
	want := []byte{0x02, 'd'}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Inject bytes = %v, want %v", got, want)
	}

	// Unicode escape: "\u00e9" → UTF-8 é.
	item = newFakeItem()
	if res := c.Exec(item, []string{client.Name, "spawn", "shell"}); !res.OK() {
		t.Fatalf("setup spawn: %v", res.Error())
	}
	res = c.Exec(item, []string{client.Name, "at", "shell", `\u00e9`})
	if !res.OK() {
		t.Fatalf("at with unicode escape returned %v, want Ok", res.Error())
	}
	if len(item.clients.injected) != 1 {
		t.Fatalf("Inject called %d times, want 1", len(item.clients.injected))
	}
	if string(item.clients.injected[0].bytes) != "é" {
		t.Fatalf("Inject bytes = %q, want %q", item.clients.injected[0].bytes, "é")
	}
}

func TestAtRejectsMalformedEscape(t *testing.T) {
	c := lookupClient(t)
	item := newFakeItem()
	if res := c.Exec(item, []string{client.Name, "spawn", "shell"}); !res.OK() {
		t.Fatalf("setup spawn: %v", res.Error())
	}
	res := c.Exec(item, []string{client.Name, "at", "shell", `\x`})
	if res.OK() {
		t.Fatalf("at with malformed escape returned Ok, want Err")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Name != "bytes" {
		t.Fatalf("ParseError.Name = %q, want %q", perr.Name, "bytes")
	}
	if len(item.clients.injected) != 0 {
		t.Fatalf("Inject called despite parse failure: %v", item.clients.injected)
	}
}

func TestAtRequiresName(t *testing.T) {
	c := lookupClient(t)
	item := newFakeItem()
	res := c.Exec(item, []string{client.Name, "at"})
	if res.OK() {
		t.Fatalf("at without name returned Ok, want Err")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Name != "name" {
		t.Fatalf("ParseError.Name = %q, want %q", perr.Name, "name")
	}
}

func TestAtRequiresBytes(t *testing.T) {
	c := lookupClient(t)
	item := newFakeItem()
	res := c.Exec(item, []string{client.Name, "at", "shell"})
	if res.OK() {
		t.Fatalf("at without bytes returned Ok, want Err")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Name != "bytes" {
		t.Fatalf("ParseError.Name = %q, want %q", perr.Name, "bytes")
	}
}

func TestAtUnknownNameIsNotFound(t *testing.T) {
	c := lookupClient(t)
	item := newFakeItem()
	res := c.Exec(item, []string{client.Name, "at", "ghost", "hi"})
	if res.OK() {
		t.Fatalf("at with unknown name returned Ok, want Err")
	}
	if !errors.Is(res.Error(), cmd.ErrNotFound) {
		t.Fatalf("at error does not wrap ErrNotFound: %v", res.Error())
	}
	if len(item.clients.injected) != 0 {
		t.Fatalf("Inject called with no stored ref: %v", item.clients.injected)
	}
}

func TestAtStaleRefClearsOption(t *testing.T) {
	c := lookupClient(t)
	item := newFakeItem()
	if res := c.Exec(item, []string{client.Name, "spawn", "shell"}); !res.OK() {
		t.Fatalf("setup spawn: %v", res.Error())
	}
	ref := item.opts.GetString(client.OptionPrefix + "shell")
	// Simulate the client having exited — option still points at it,
	// but fakeClients no longer tracks it as live.
	delete(item.clients.live, ref)

	res := c.Exec(item, []string{client.Name, "at", "shell", "hi"})
	if res.OK() {
		t.Fatalf("at against stale ref returned Ok, want Err")
	}
	if !errors.Is(res.Error(), cmd.ErrStaleClient) {
		t.Fatalf("at error does not wrap ErrStaleClient: %v", res.Error())
	}
	if got := item.opts.GetString(client.OptionPrefix + "shell"); got != "" {
		t.Fatalf("@client/shell = %q after stale-ref at, want empty", got)
	}
}
