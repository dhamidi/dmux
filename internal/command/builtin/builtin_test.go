package builtin_test

import (
	"fmt"
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

	// Buffer recording.
	buffers       map[string]string
	loadedBuffers []struct{ name, path string }
	savedBuffers  []struct{ name, path string }
	pastedBuffers []struct {
		name   string
		paneID int
	}

	// Pane mutation recording.
	resizedPanes []struct {
		paneID    int
		direction string
		amount    int
	}
	capturedPanes []struct {
		paneID  int
		history bool
	}
	captureOutput string
	respawnedPanes []struct {
		paneID int
		shell  string
	}

	// Window/pane movement recording.
	movedWindows []struct {
		sessionID, windowID string
		newIndex            int
	}
	swappedWindows []struct {
		sessionID, aWindowID, bWindowID string
	}
	foundWindows    []struct{ sessionID, pattern string }
	foundWindowView command.WindowView
	swappedPanes    []struct {
		sessionID, windowID string
		paneA, paneB        int
	}
	brokenPanes []struct {
		sessionID, windowID string
		paneID              int
	}
	brokenWindowView command.WindowView
	joinedPanes      []struct {
		srcSessionID, srcWindowID string
		srcPaneID                 int
		dstSessionID, dstWindowID string
	}

	// Environment recording.
	environ         map[string]map[string]string // scope -> name -> value
	serverMessages  []string
	lockServerCalled bool
	signaledChannels []string
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

func (b *testBackend) SetBuffer(name, data string) error {
	if b.buffers == nil {
		b.buffers = make(map[string]string)
	}
	if name == "" {
		name = fmt.Sprintf("buffer%d", len(b.buffers))
	}
	b.buffers[name] = data
	return nil
}

func (b *testBackend) DeleteBuffer(name string) error {
	if _, ok := b.buffers[name]; !ok {
		return fmt.Errorf("buffer %q not found", name)
	}
	delete(b.buffers, name)
	return nil
}

func (b *testBackend) LoadBuffer(name, path string) error {
	b.loadedBuffers = append(b.loadedBuffers, struct{ name, path string }{name, path})
	return nil
}

func (b *testBackend) SaveBuffer(name, path string) error {
	if _, ok := b.buffers[name]; !ok {
		return fmt.Errorf("buffer %q not found", name)
	}
	b.savedBuffers = append(b.savedBuffers, struct{ name, path string }{name, path})
	return nil
}

func (b *testBackend) PasteBuffer(name string, paneID int) error {
	if _, ok := b.buffers[name]; !ok {
		return fmt.Errorf("buffer %q not found", name)
	}
	b.pastedBuffers = append(b.pastedBuffers, struct {
		name   string
		paneID int
	}{name, paneID})
	return nil
}

func (b *testBackend) ListBuffers() []command.BufferEntry {
	var out []command.BufferEntry
	for name, data := range b.buffers {
		out = append(out, command.BufferEntry{Name: name, Size: len(data)})
	}
	return out
}

func (b *testBackend) ResizePane(paneID int, direction string, amount int) error {
	b.resizedPanes = append(b.resizedPanes, struct {
		paneID    int
		direction string
		amount    int
	}{paneID, direction, amount})
	return nil
}

func (b *testBackend) CapturePane(paneID int, history bool) (string, error) {
	b.capturedPanes = append(b.capturedPanes, struct {
		paneID  int
		history bool
	}{paneID, history})
	return b.captureOutput, nil
}

func (b *testBackend) RespawnPane(paneID int, shell string) error {
	b.respawnedPanes = append(b.respawnedPanes, struct {
		paneID int
		shell  string
	}{paneID, shell})
	return nil
}

func (b *testBackend) MoveWindow(sessionID, windowID string, newIndex int) error {
	b.movedWindows = append(b.movedWindows, struct {
		sessionID, windowID string
		newIndex            int
	}{sessionID, windowID, newIndex})
	return nil
}

func (b *testBackend) SwapWindows(sessionID, aWindowID, bWindowID string) error {
	b.swappedWindows = append(b.swappedWindows, struct {
		sessionID, aWindowID, bWindowID string
	}{sessionID, aWindowID, bWindowID})
	return nil
}

func (b *testBackend) FindWindow(sessionID, pattern string) (command.WindowView, error) {
	b.foundWindows = append(b.foundWindows, struct{ sessionID, pattern string }{sessionID, pattern})
	return b.foundWindowView, nil
}

func (b *testBackend) SwapPane(sessionID, windowID string, paneA, paneB int) error {
	b.swappedPanes = append(b.swappedPanes, struct {
		sessionID, windowID string
		paneA, paneB        int
	}{sessionID, windowID, paneA, paneB})
	return nil
}

func (b *testBackend) BreakPane(sessionID, windowID string, paneID int) (command.WindowView, error) {
	b.brokenPanes = append(b.brokenPanes, struct {
		sessionID, windowID string
		paneID              int
	}{sessionID, windowID, paneID})
	return b.brokenWindowView, nil
}

func (b *testBackend) JoinPane(srcSessionID, srcWindowID string, srcPaneID int, dstSessionID, dstWindowID string) error {
	b.joinedPanes = append(b.joinedPanes, struct {
		srcSessionID, srcWindowID string
		srcPaneID                 int
		dstSessionID, dstWindowID string
	}{srcSessionID, srcWindowID, srcPaneID, dstSessionID, dstWindowID})
	return nil
}

func (b *testBackend) SetEnvironment(scope, name, value string, remove bool) error {
	if b.environ == nil {
		b.environ = make(map[string]map[string]string)
	}
	if b.environ[scope] == nil {
		b.environ[scope] = make(map[string]string)
	}
	if remove {
		delete(b.environ[scope], name)
	} else {
		b.environ[scope][name] = value
	}
	return nil
}

func (b *testBackend) ListEnvironment(scope string) []command.EnvironEntry {
	if b.environ == nil || b.environ[scope] == nil {
		return nil
	}
	var out []command.EnvironEntry
	for k, v := range b.environ[scope] {
		out = append(out, command.EnvironEntry{Name: k, Value: v})
	}
	return out
}

func (b *testBackend) ShowMessages() []string {
	return b.serverMessages
}

func (b *testBackend) LockServer() error {
	b.lockServerCalled = true
	return nil
}

func (b *testBackend) WaitFor(channel string) error {
	// In tests, WaitFor returns immediately (no blocking).
	return nil
}

func (b *testBackend) SignalChannel(channel string) {
	b.signaledChannels = append(b.signaledChannels, channel)
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

func TestResizePane_ForwardsDirectionAndAmount(t *testing.T) {
	b := newBackend()
	res := dispatch("resize-pane", []string{"-R", "5"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.resizedPanes) != 1 {
		t.Fatalf("expected 1 resize call, got %d", len(b.resizedPanes))
	}
	got := b.resizedPanes[0]
	if got.paneID != 1 {
		t.Errorf("ResizePane paneID = %d, want 1", got.paneID)
	}
	if got.direction != "R" {
		t.Errorf("ResizePane direction = %q, want %q", got.direction, "R")
	}
	if got.amount != 5 {
		t.Errorf("ResizePane amount = %d, want 5", got.amount)
	}
}

func TestResizePane_DefaultAmount(t *testing.T) {
	b := newBackend()
	res := dispatch("resize-pane", []string{"-D"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.resizedPanes) != 1 || b.resizedPanes[0].amount != 1 {
		t.Errorf("expected default amount 1, got: %v", b.resizedPanes)
	}
}

func TestCapturePane_PrintsToOutput(t *testing.T) {
	b := newBackend()
	b.captureOutput = "hello world\n"
	res := dispatch("capture-pane", []string{"-p"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.Output != "hello world\n" {
		t.Errorf("capture-pane -p output = %q, want %q", res.Output, "hello world\n")
	}
	if len(b.capturedPanes) != 1 || b.capturedPanes[0].paneID != 1 {
		t.Errorf("CapturePane not called with paneID=1: %v", b.capturedPanes)
	}
}

func TestCapturePane_StoresInBuffer(t *testing.T) {
	b := newBackend()
	b.captureOutput = "content"
	res := dispatch("capture-pane", []string{"-b", "mybuf"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if b.buffers["mybuf"] != "content" {
		t.Errorf("buffer 'mybuf' = %q, want %q", b.buffers["mybuf"], "content")
	}
}

func TestRespawnPane_ForwardsPaneIDAndShell(t *testing.T) {
	b := newBackend()
	res := dispatch("respawn-pane", []string{"-e", "/bin/bash"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.respawnedPanes) != 1 {
		t.Fatalf("expected 1 respawn call, got %d", len(b.respawnedPanes))
	}
	got := b.respawnedPanes[0]
	if got.paneID != 1 {
		t.Errorf("RespawnPane paneID = %d, want 1", got.paneID)
	}
	if got.shell != "/bin/bash" {
		t.Errorf("RespawnPane shell = %q, want %q", got.shell, "/bin/bash")
	}
}

// ─── Movement command tests ───────────────────────────────────────────────────

func newBackendTwoWindows() *testBackend {
	b := newBackend()
	win2 := command.WindowView{ID: "w2", Name: "other", Index: 1, Panes: []command.PaneView{{ID: 2, Title: "sh"}}, Active: 2}
	s := b.sessions[0]
	s.Windows = append(s.Windows, win2)
	b.sessions[0] = s
	return b
}

func TestMoveWindow_MovesToEnd(t *testing.T) {
	b := newBackend()
	res := dispatch("move-window", []string{"-t", "alpha:main", "-a"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.movedWindows) != 1 {
		t.Fatalf("expected 1 MoveWindow call, got %d", len(b.movedWindows))
	}
	got := b.movedWindows[0]
	if got.sessionID != "s1" || got.windowID != "w1" {
		t.Errorf("MoveWindow(%q, %q, _): unexpected session/window", got.sessionID, got.windowID)
	}
	if got.newIndex != -1 {
		t.Errorf("MoveWindow newIndex = %d, want -1 (append)", got.newIndex)
	}
}

func TestSwapWindow_SwapsTwoWindows(t *testing.T) {
	b := newBackendTwoWindows()
	res := dispatch("swap-window", []string{"-t", "alpha:main", "-s", "other"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.swappedWindows) != 1 {
		t.Fatalf("expected 1 SwapWindows call, got %d", len(b.swappedWindows))
	}
	got := b.swappedWindows[0]
	if got.sessionID != "s1" {
		t.Errorf("SwapWindows sessionID = %q, want %q", got.sessionID, "s1")
	}
	if got.aWindowID != "w1" || got.bWindowID != "w2" {
		t.Errorf("SwapWindows(%q, %q): unexpected window IDs", got.aWindowID, got.bWindowID)
	}
}

func TestFindWindow_ReturnsMatchingWindow(t *testing.T) {
	b := newBackend()
	b.foundWindowView = command.WindowView{ID: "w1", Name: "main", Index: 0}
	res := dispatch("find-window", []string{"main"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.foundWindows) != 1 {
		t.Fatalf("expected 1 FindWindow call, got %d", len(b.foundWindows))
	}
	if b.foundWindows[0].pattern != "main" {
		t.Errorf("FindWindow pattern = %q, want %q", b.foundWindows[0].pattern, "main")
	}
	if !strings.Contains(res.Output, "main") {
		t.Errorf("find-window output %q does not contain 'main'", res.Output)
	}
}

func TestSwapPane_SwapsTwoPanes(t *testing.T) {
	b := newBackend()
	res := dispatch("swap-pane", []string{"-t", "alpha:main.%1", "-s", "2"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.swappedPanes) != 1 {
		t.Fatalf("expected 1 SwapPane call, got %d", len(b.swappedPanes))
	}
	got := b.swappedPanes[0]
	if got.paneA != 1 || got.paneB != 2 {
		t.Errorf("SwapPane(%d, %d): want (1, 2)", got.paneA, got.paneB)
	}
}

func TestBreakPane_DetachesActivePane(t *testing.T) {
	b := newBackend()
	b.brokenWindowView = command.WindowView{ID: "w2", Name: "bash", Index: 1}
	res := dispatch("break-pane", nil, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.brokenPanes) != 1 {
		t.Fatalf("expected 1 BreakPane call, got %d", len(b.brokenPanes))
	}
	got := b.brokenPanes[0]
	if got.sessionID != "s1" || got.windowID != "w1" || got.paneID != 1 {
		t.Errorf("BreakPane(%q, %q, %d): unexpected args", got.sessionID, got.windowID, got.paneID)
	}
}

func TestBreakPane_PrintFlag_OutputsWindowInfo(t *testing.T) {
	b := newBackend()
	b.brokenWindowView = command.WindowView{ID: "w2", Name: "bash", Index: 1}
	res := dispatch("break-pane", []string{"-P"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !strings.Contains(res.Output, "bash") {
		t.Errorf("break-pane -P output %q does not contain 'bash'", res.Output)
	}
}

func TestJoinPane_MovesPaneBetweenWindows(t *testing.T) {
	b := newBackendTwoWindows()
	// Move pane 1 from window index 0 ("main") into window index 1 ("other").
	res := dispatch("join-pane", []string{"-s", ":0.1", "-t", "alpha:other"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.joinedPanes) != 1 {
		t.Fatalf("expected 1 JoinPane call, got %d", len(b.joinedPanes))
	}
	got := b.joinedPanes[0]
	if got.srcWindowID != "w1" {
		t.Errorf("JoinPane srcWindowID = %q, want %q", got.srcWindowID, "w1")
	}
	if got.srcPaneID != 1 {
		t.Errorf("JoinPane srcPaneID = %d, want 1", got.srcPaneID)
	}
	if got.dstWindowID != "w2" {
		t.Errorf("JoinPane dstWindowID = %q, want %q", got.dstWindowID, "w2")
	}
}

// ─── Environment and server management tests ──────────────────────────────────

func TestSetEnvironment_StoresValue(t *testing.T) {
	b := newBackend()
	res := dispatch("set-environment", []string{"FOO", "bar"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	// Default scope is the current session ID "s1".
	if b.environ["s1"]["FOO"] != "bar" {
		t.Errorf("environ[s1][FOO] = %q, want %q", b.environ["s1"]["FOO"], "bar")
	}
}

func TestSetEnvironment_GlobalScope(t *testing.T) {
	b := newBackend()
	res := dispatch("set-environment", []string{"-g", "GLOBAL", "value"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if b.environ["global"]["GLOBAL"] != "value" {
		t.Errorf("environ[global][GLOBAL] = %q, want %q", b.environ["global"]["GLOBAL"], "value")
	}
}

func TestSetEnvironment_RemovesVariable(t *testing.T) {
	b := newBackend()
	// Pre-populate.
	if err := b.SetEnvironment("s1", "FOO", "bar", false); err != nil {
		t.Fatal(err)
	}
	res := dispatch("set-environment", []string{"-r", "FOO"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if _, ok := b.environ["s1"]["FOO"]; ok {
		t.Error("FOO was not removed from environ")
	}
}

func TestShowEnvironment_FormatsOutput(t *testing.T) {
	b := newBackend()
	if err := b.SetEnvironment("s1", "FOO", "bar", false); err != nil {
		t.Fatal(err)
	}
	res := dispatch("show-environment", nil, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !strings.Contains(res.Output, "FOO=bar") {
		t.Errorf("show-environment output %q does not contain 'FOO=bar'", res.Output)
	}
}

func TestShowEnvironment_FiltersByName(t *testing.T) {
	b := newBackend()
	if err := b.SetEnvironment("s1", "FOO", "bar", false); err != nil {
		t.Fatal(err)
	}
	if err := b.SetEnvironment("s1", "BAZ", "qux", false); err != nil {
		t.Fatal(err)
	}
	res := dispatch("show-environment", []string{"FOO"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !strings.Contains(res.Output, "FOO=bar") {
		t.Errorf("output %q missing 'FOO=bar'", res.Output)
	}
	if strings.Contains(res.Output, "BAZ") {
		t.Errorf("output %q should not contain 'BAZ'", res.Output)
	}
}

func TestShowMessages_ReturnsMessages(t *testing.T) {
	b := newBackend()
	b.serverMessages = []string{"hello", "world"}
	res := dispatch("show-messages", nil, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !strings.Contains(res.Output, "hello") {
		t.Errorf("show-messages output %q does not contain 'hello'", res.Output)
	}
	if !strings.Contains(res.Output, "world") {
		t.Errorf("show-messages output %q does not contain 'world'", res.Output)
	}
}

func TestWaitFor_Signal_SignalsChannel(t *testing.T) {
	b := newBackend()
	res := dispatch("wait-for", []string{"-S", "mychan"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.signaledChannels) != 1 || b.signaledChannels[0] != "mychan" {
		t.Errorf("SignalChannel not called with 'mychan': %v", b.signaledChannels)
	}
}

func TestLockServer_CallsMutator(t *testing.T) {
	b := newBackend()
	res := dispatch("lock-server", nil, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !b.lockServerCalled {
		t.Error("LockServer() was not called")
	}
}

func TestStartServer_ReturnsOK(t *testing.T) {
	b := newBackend()
	res := dispatch("start-server", nil, b)
	if res.Err != nil {
		t.Fatalf("start-server returned error: %v", res.Err)
	}
}
