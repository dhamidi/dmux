package builtin_test

import (
	"strings"
	"testing"

	_ "github.com/dhamidi/dmux/internal/command/builtin" // register all builtins
	"github.com/dhamidi/dmux/internal/command"
)

// ─── Test backend stub ────────────────────────────────────────────────────────

// testBackend implements command.Server and command.Mutator for tests.
type testBackend struct {
	// Server (read-only) state.
	sessions []command.SessionView
	clients  []command.ClientView

	// Mutation recording.
	newSessionCalls   []string
	killedSessions    []string
	renamedSessions   map[string]string
	detachedClients   []string
	killedWindows     [][2]string
	renamedWindows    map[[2]string]string
	selectedWindows   [][2]string
	splitWindows      [][2]string
	killedPanes       []int
	selectedPanes     []struct {
		sess, win string
		pane      int
	}
	boundKeys   []struct{ table, key, cmd string }
	unboundKeys [][2]string
	keyBindings []command.KeyBinding
	setOptions  []struct{ scope, name, value string }
	unsetOptions  [][2]string
	optionEntries []command.OptionEntry
	serverKilled      bool
	displayedMessages []struct{ clientID, msg string }
	sentKeys          []struct {
		paneID int
		keys   []string
	}
	shellOutput string
	shellErr    error
}

// ─── command.Server (read) implementation ────────────────────────────────────

func (b *testBackend) GetSession(id string) (command.SessionView, bool) {
	for _, s := range b.sessions {
		if s.ID == id {
			return s, true
		}
	}
	return command.SessionView{}, false
}

func (b *testBackend) GetSessionByName(name string) (command.SessionView, bool) {
	for _, s := range b.sessions {
		if s.Name == name {
			return s, true
		}
	}
	return command.SessionView{}, false
}

func (b *testBackend) ListSessions() []command.SessionView { return b.sessions }

func (b *testBackend) GetClient(id string) (command.ClientView, bool) {
	for _, c := range b.clients {
		if c.ID == id {
			return c, true
		}
	}
	return command.ClientView{}, false
}

func (b *testBackend) ListClients() []command.ClientView { return b.clients }

// ─── command.Mutator implementation ──────────────────────────────────────────

func (b *testBackend) NewSession(name string) (command.SessionView, error) {
	b.newSessionCalls = append(b.newSessionCalls, name)
	s := command.SessionView{ID: "new-" + name, Name: name}
	return s, nil
}

func (b *testBackend) KillSession(id string) error {
	b.killedSessions = append(b.killedSessions, id)
	return nil
}

func (b *testBackend) RenameSession(id, name string) error {
	if b.renamedSessions == nil {
		b.renamedSessions = make(map[string]string)
	}
	b.renamedSessions[id] = name
	return nil
}

func (b *testBackend) AttachClient(clientID, sessionID string) error { return nil }

func (b *testBackend) DetachClient(clientID string) error {
	b.detachedClients = append(b.detachedClients, clientID)
	return nil
}

func (b *testBackend) SwitchClient(clientID, sessionID string) error { return nil }

func (b *testBackend) NewWindow(sessionID, name string) (command.WindowView, error) {
	return command.WindowView{ID: "new-win", Name: name}, nil
}

func (b *testBackend) KillWindow(sessionID, windowID string) error {
	b.killedWindows = append(b.killedWindows, [2]string{sessionID, windowID})
	return nil
}

func (b *testBackend) RenameWindow(sessionID, windowID, name string) error {
	if b.renamedWindows == nil {
		b.renamedWindows = make(map[[2]string]string)
	}
	b.renamedWindows[[2]string{sessionID, windowID}] = name
	return nil
}

func (b *testBackend) SelectWindow(sessionID, windowID string) error {
	b.selectedWindows = append(b.selectedWindows, [2]string{sessionID, windowID})
	return nil
}

func (b *testBackend) SplitWindow(sessionID, windowID string) (command.PaneView, error) {
	b.splitWindows = append(b.splitWindows, [2]string{sessionID, windowID})
	return command.PaneView{ID: 99, Title: "new"}, nil
}

func (b *testBackend) KillPane(paneID int) error {
	b.killedPanes = append(b.killedPanes, paneID)
	return nil
}

func (b *testBackend) SelectPane(sessionID, windowID string, paneID int) error {
	b.selectedPanes = append(b.selectedPanes, struct {
		sess, win string
		pane      int
	}{sessionID, windowID, paneID})
	return nil
}

func (b *testBackend) BindKey(table, key, cmd string) error {
	b.boundKeys = append(b.boundKeys, struct{ table, key, cmd string }{table, key, cmd})
	return nil
}

func (b *testBackend) UnbindKey(table, key string) error {
	b.unboundKeys = append(b.unboundKeys, [2]string{table, key})
	return nil
}

func (b *testBackend) ListKeyBindings(table string) []command.KeyBinding {
	if table == "" {
		return b.keyBindings
	}
	var out []command.KeyBinding
	for _, kb := range b.keyBindings {
		if kb.Table == table {
			out = append(out, kb)
		}
	}
	return out
}

func (b *testBackend) SetOption(scope, name, value string) error {
	b.setOptions = append(b.setOptions, struct{ scope, name, value string }{scope, name, value})
	return nil
}

func (b *testBackend) UnsetOption(scope, name string) error {
	b.unsetOptions = append(b.unsetOptions, [2]string{scope, name})
	return nil
}

func (b *testBackend) ListOptions(scope string) []command.OptionEntry {
	return b.optionEntries
}

func (b *testBackend) KillServer() error {
	b.serverKilled = true
	return nil
}

func (b *testBackend) DisplayMessage(clientID, msg string) error {
	b.displayedMessages = append(b.displayedMessages, struct{ clientID, msg string }{clientID, msg})
	return nil
}

func (b *testBackend) SendKeys(paneID int, keys []string) error {
	b.sentKeys = append(b.sentKeys, struct {
		paneID int
		keys   []string
	}{paneID, keys})
	return nil
}

func (b *testBackend) RunShell(cmd string, background bool) (string, error) {
	return b.shellOutput, b.shellErr
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func newBackend() *testBackend {
	pane := command.PaneView{ID: 1, Title: "bash"}
	win := command.WindowView{
		ID: "w1", Name: "main", Index: 0,
		Panes:  []command.PaneView{pane},
		Active: 1,
	}
	sess := command.SessionView{
		ID: "s1", Name: "alpha",
		Windows: []command.WindowView{win},
		Current: 0,
	}
	return &testBackend{
		sessions: []command.SessionView{sess},
		clients:  []command.ClientView{{ID: "c1", SessionID: "s1", KeyTable: "root"}},
	}
}

func client1() command.ClientView {
	return command.ClientView{ID: "c1", SessionID: "s1", KeyTable: "root"}
}

func dispatch(name string, args []string, b *testBackend) command.Result {
	return command.Default.Dispatch(name, args, b, client1(), command.NewQueue(), b)
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestListSessions_OutputsAllSessions(t *testing.T) {
	b := newBackend()
	res := dispatch("list-sessions", nil, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !strings.Contains(res.Output, "alpha") {
		t.Errorf("output %q does not contain session name 'alpha'", res.Output)
	}
}

func TestNewSession_CallsMutator(t *testing.T) {
	b := newBackend()
	res := dispatch("new-session", []string{"-s", "mysess"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.newSessionCalls) != 1 || b.newSessionCalls[0] != "mysess" {
		t.Errorf("NewSession not called with 'mysess', got: %v", b.newSessionCalls)
	}
}

func TestKillSession_KillsTargetSession(t *testing.T) {
	b := newBackend()
	res := dispatch("kill-session", []string{"-t", "alpha"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.killedSessions) != 1 || b.killedSessions[0] != "s1" {
		t.Errorf("KillSession(%q) not called; got: %v", "s1", b.killedSessions)
	}
}

func TestRenameSession_RenamesTarget(t *testing.T) {
	b := newBackend()
	res := dispatch("rename-session", []string{"-t", "alpha", "beta"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if b.renamedSessions["s1"] != "beta" {
		t.Errorf("RenameSession(s1, beta) not called; got: %v", b.renamedSessions)
	}
}

func TestKillServer_SetsFlag(t *testing.T) {
	b := newBackend()
	res := dispatch("kill-server", nil, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !b.serverKilled {
		t.Error("KillServer() was not called")
	}
}

func TestBindKey_RecordsBinding(t *testing.T) {
	b := newBackend()
	res := dispatch("bind-key", []string{"-T", "root", "C-b", "new-session"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.boundKeys) != 1 {
		t.Fatalf("expected 1 bound key, got %d", len(b.boundKeys))
	}
	got := b.boundKeys[0]
	if got.table != "root" || got.key != "C-b" || got.cmd != "new-session" {
		t.Errorf("BindKey(%q, %q, %q); got table=%q key=%q cmd=%q",
			"root", "C-b", "new-session", got.table, got.key, got.cmd)
	}
}

func TestSetOption_RecordsOption(t *testing.T) {
	b := newBackend()
	res := dispatch("set-option", []string{"-g", "status", "on"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.setOptions) != 1 {
		t.Fatalf("expected 1 set option, got %d", len(b.setOptions))
	}
	got := b.setOptions[0]
	if got.scope != "global" || got.name != "status" || got.value != "on" {
		t.Errorf("SetOption(%q, %q, %q); got %+v", "global", "status", "on", got)
	}
}

func TestListCommands_ContainsBuiltins(t *testing.T) {
	b := newBackend()
	res := dispatch("list-commands", nil, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	for _, expected := range []string{"new-session", "kill-server", "bind-key", "list-sessions"} {
		if !strings.Contains(res.Output, expected) {
			t.Errorf("list-commands output missing %q:\n%s", expected, res.Output)
		}
	}
}

func TestListWindows_OutputsWindows(t *testing.T) {
	b := newBackend()
	res := dispatch("list-windows", []string{"-t", "alpha"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !strings.Contains(res.Output, "main") {
		t.Errorf("list-windows output missing window 'main': %q", res.Output)
	}
}

func TestListClients_OutputsClients(t *testing.T) {
	b := newBackend()
	res := dispatch("list-clients", nil, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !strings.Contains(res.Output, "c1") {
		t.Errorf("list-clients output missing client 'c1': %q", res.Output)
	}
}

func TestUnbindKey_RecordsUnbind(t *testing.T) {
	b := newBackend()
	res := dispatch("unbind-key", []string{"-T", "prefix", "C-c"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.unboundKeys) != 1 || b.unboundKeys[0] != [2]string{"prefix", "C-c"} {
		t.Errorf("UnbindKey not recorded correctly: %v", b.unboundKeys)
	}
}

func TestKillPane_KillsTargetPane(t *testing.T) {
	b := newBackend()
	// Target alpha:main.%1
	res := dispatch("kill-pane", []string{"-t", "alpha:main.%1"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.killedPanes) != 1 || b.killedPanes[0] != 1 {
		t.Errorf("KillPane(1) not called; got: %v", b.killedPanes)
	}
}
