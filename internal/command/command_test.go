package command_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/dhamidi/dmux/internal/command"
)

// ─── Stub server ─────────────────────────────────────────────────────────────

// stubServer implements command.Server for tests without a live session.Server.
type stubServer struct {
	sessions []command.SessionView
	clients  []command.ClientView
}

func (s *stubServer) GetSession(id string) (command.SessionView, bool) {
	for _, sess := range s.sessions {
		if sess.ID == id {
			return sess, true
		}
	}
	return command.SessionView{}, false
}

func (s *stubServer) GetSessionByName(name string) (command.SessionView, bool) {
	for _, sess := range s.sessions {
		if sess.Name == name {
			return sess, true
		}
	}
	return command.SessionView{}, false
}

func (s *stubServer) ListSessions() []command.SessionView { return s.sessions }

func (s *stubServer) GetClient(id string) (command.ClientView, bool) {
	for _, c := range s.clients {
		if c.ID == id {
			return c, true
		}
	}
	return command.ClientView{}, false
}

func (s *stubServer) ListClients() []command.ClientView { return s.clients }

// ─── Helpers ──────────────────────────────────────────────────────────────────

func mustRegister(t *testing.T, r *command.Registry, spec command.Spec) {
	t.Helper()
	if err := r.Register(spec); err != nil {
		t.Fatalf("Register(%q): %v", spec.Name, err)
	}
}

// singleSession builds a stub server with one session containing one window
// with one pane.
func singleSession() (*stubServer, command.SessionView) {
	pane := command.PaneView{ID: 1, Title: "bash"}
	win := command.WindowView{
		ID: "w1", Name: "main", Index: 1,
		Panes:  []command.PaneView{pane},
		Active: 1,
	}
	sess := command.SessionView{
		ID: "s1", Name: "alpha",
		Windows: []command.WindowView{win},
		Current: 0,
	}
	return &stubServer{sessions: []command.SessionView{sess}}, sess
}

// attachedClient returns a ClientView attached to sessionID.
func attachedClient(id, sessionID string) command.ClientView {
	return command.ClientView{ID: id, SessionID: sessionID, KeyTable: "root"}
}

// ─── Registry tests ───────────────────────────────────────────────────────────

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := command.NewRegistry()
	mustRegister(t, r, command.Spec{
		Name: "hello",
		Run:  func(*command.Ctx) command.Result { return command.OK() },
	})

	spec := r.Lookup("hello")
	if spec == nil {
		t.Fatal("Lookup(\"hello\") returned nil")
	}
	if spec.Name != "hello" {
		t.Errorf("spec.Name = %q, want %q", spec.Name, "hello")
	}
}

func TestRegistry_LookupMissing(t *testing.T) {
	r := command.NewRegistry()
	if got := r.Lookup("does-not-exist"); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestRegistry_AliasLookup(t *testing.T) {
	r := command.NewRegistry()
	mustRegister(t, r, command.Spec{
		Name:  "new-session",
		Alias: []string{"new-s", "ns"},
		Run:   func(*command.Ctx) command.Result { return command.OK() },
	})

	for _, name := range []string{"new-session", "new-s", "ns"} {
		if r.Lookup(name) == nil {
			t.Errorf("Lookup(%q) = nil, want non-nil", name)
		}
	}
}

func TestRegistry_DuplicateReturnsError(t *testing.T) {
	r := command.NewRegistry()
	spec := command.Spec{Name: "foo", Run: func(*command.Ctx) command.Result { return command.OK() }}
	if err := r.Register(spec); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := r.Register(spec); err == nil {
		t.Error("second Register: expected error, got nil")
	}
}

func TestRegistry_List(t *testing.T) {
	r := command.NewRegistry()
	mustRegister(t, r, command.Spec{
		Name:  "foo",
		Alias: []string{"f"},
		Run:   func(*command.Ctx) command.Result { return command.OK() },
	})
	mustRegister(t, r, command.Spec{
		Name: "bar",
		Run:  func(*command.Ctx) command.Result { return command.OK() },
	})

	specs := r.List()
	if len(specs) != 2 {
		t.Errorf("List() returned %d specs, want 2", len(specs))
	}
}

// ─── Argument parsing tests ───────────────────────────────────────────────────

func TestParseArgs_BooleanFlag(t *testing.T) {
	r := command.NewRegistry()
	mustRegister(t, r, command.Spec{
		Name: "kill-session",
		Args: command.ArgSpec{Flags: []string{"d"}},
		Run: func(ctx *command.Ctx) command.Result {
			if !ctx.Args.Flag("d") {
				return command.Errorf("-d not set")
			}
			return command.OK()
		},
	})

	store, _ := singleSession()
	client := attachedClient("c1", "s1")
	res := r.Dispatch("kill-session", []string{"-d"}, store, client, command.NewQueue())
	if res.Err != nil {
		t.Errorf("unexpected error: %v", res.Err)
	}
}

func TestParseArgs_CombinedFlags(t *testing.T) {
	r := command.NewRegistry()
	var gotD, gotP bool
	mustRegister(t, r, command.Spec{
		Name: "split-window",
		Args: command.ArgSpec{Flags: []string{"d", "P"}},
		Run: func(ctx *command.Ctx) command.Result {
			gotD = ctx.Args.Flag("d")
			gotP = ctx.Args.Flag("P")
			return command.OK()
		},
	})

	store, _ := singleSession()
	client := attachedClient("c1", "s1")
	r.Dispatch("split-window", []string{"-dP"}, store, client, command.NewQueue())
	if !gotD || !gotP {
		t.Errorf("combined -dP: gotD=%v gotP=%v, want both true", gotD, gotP)
	}
}

func TestParseArgs_StringOption(t *testing.T) {
	r := command.NewRegistry()
	var gotName string
	mustRegister(t, r, command.Spec{
		Name: "new-session",
		Args: command.ArgSpec{Options: []string{"s"}},
		Run: func(ctx *command.Ctx) command.Result {
			gotName = ctx.Args.Option("s")
			return command.OK()
		},
	})

	store, _ := singleSession()
	r.Dispatch("new-session", []string{"-s", "mysession"}, store, command.ClientView{}, command.NewQueue())
	if gotName != "mysession" {
		t.Errorf("-s value = %q, want %q", gotName, "mysession")
	}
}

func TestParseArgs_StringOptionConcatenated(t *testing.T) {
	r := command.NewRegistry()
	var gotName string
	mustRegister(t, r, command.Spec{
		Name: "rename-session",
		Args: command.ArgSpec{Options: []string{"t"}},
		Run: func(ctx *command.Ctx) command.Result {
			gotName = ctx.Args.Option("t")
			return command.OK()
		},
	})

	store, _ := singleSession()
	r.Dispatch("rename-session", []string{"-talpha"}, store, command.ClientView{}, command.NewQueue())
	if gotName != "alpha" {
		t.Errorf("-t value = %q, want %q", gotName, "alpha")
	}
}

func TestParseArgs_PositionalArgs(t *testing.T) {
	r := command.NewRegistry()
	var gotArgs []string
	mustRegister(t, r, command.Spec{
		Name: "send-keys",
		Args: command.ArgSpec{MinArgs: 1, MaxArgs: -1},
		Run: func(ctx *command.Ctx) command.Result {
			gotArgs = ctx.Args.Positional
			return command.OK()
		},
	})

	store, _ := singleSession()
	r.Dispatch("send-keys", []string{"Enter", "q"}, store, command.ClientView{}, command.NewQueue())
	if len(gotArgs) != 2 || gotArgs[0] != "Enter" || gotArgs[1] != "q" {
		t.Errorf("Positional = %v, want [Enter q]", gotArgs)
	}
}

func TestParseArgs_TooFewPositional(t *testing.T) {
	r := command.NewRegistry()
	mustRegister(t, r, command.Spec{
		Name: "rename-session",
		Args: command.ArgSpec{MinArgs: 1, MaxArgs: 1},
		Run:  func(*command.Ctx) command.Result { return command.OK() },
	})

	store, _ := singleSession()
	res := r.Dispatch("rename-session", nil, store, command.ClientView{}, command.NewQueue())
	if res.Err == nil {
		t.Error("expected error for too few args, got nil")
	}
}

func TestParseArgs_TooManyPositional(t *testing.T) {
	r := command.NewRegistry()
	mustRegister(t, r, command.Spec{
		Name: "new-session",
		Args: command.ArgSpec{MaxArgs: 0},
		Run:  func(*command.Ctx) command.Result { return command.OK() },
	})

	store, _ := singleSession()
	res := r.Dispatch("new-session", []string{"extra"}, store, command.ClientView{}, command.NewQueue())
	if res.Err == nil {
		t.Error("expected error for too many args, got nil")
	}
}

func TestParseArgs_UnknownFlag(t *testing.T) {
	r := command.NewRegistry()
	mustRegister(t, r, command.Spec{
		Name: "list-sessions",
		Run:  func(*command.Ctx) command.Result { return command.OK() },
	})

	store, _ := singleSession()
	res := r.Dispatch("list-sessions", []string{"-z"}, store, command.ClientView{}, command.NewQueue())
	if res.Err == nil {
		t.Error("expected error for unknown flag, got nil")
	}
}

func TestParseArgs_DashDashEndsFlags(t *testing.T) {
	r := command.NewRegistry()
	var got []string
	mustRegister(t, r, command.Spec{
		Name: "run-shell",
		Args: command.ArgSpec{MaxArgs: -1},
		Run: func(ctx *command.Ctx) command.Result {
			got = ctx.Args.Positional
			return command.OK()
		},
	})

	store, _ := singleSession()
	r.Dispatch("run-shell", []string{"--", "-not-a-flag"}, store, command.ClientView{}, command.NewQueue())
	if len(got) != 1 || got[0] != "-not-a-flag" {
		t.Errorf("Positional = %v, want [-not-a-flag]", got)
	}
}

// ─── Target resolution tests ──────────────────────────────────────────────────

func TestTargetResolution_DefaultFromClient(t *testing.T) {
	r := command.NewRegistry()
	var gotTarget command.Target
	mustRegister(t, r, command.Spec{
		Name:   "select-window",
		Target: command.TargetSpec{Kind: command.TargetWindow},
		Run: func(ctx *command.Ctx) command.Result {
			gotTarget = ctx.Target
			return command.OK()
		},
	})

	store, sess := singleSession()
	client := attachedClient("c1", sess.ID)
	res := r.Dispatch("select-window", nil, store, client, command.NewQueue())
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if gotTarget.Session.ID != sess.ID {
		t.Errorf("Target.Session.ID = %q, want %q", gotTarget.Session.ID, sess.ID)
	}
	if gotTarget.Window.ID == "" {
		t.Error("Target.Window is empty, expected the current window")
	}
}

func TestTargetResolution_BySessionName(t *testing.T) {
	r := command.NewRegistry()
	var gotSess command.SessionView
	mustRegister(t, r, command.Spec{
		Name:   "attach-session",
		Target: command.TargetSpec{Kind: command.TargetSession},
		Run: func(ctx *command.Ctx) command.Result {
			gotSess = ctx.Target.Session
			return command.OK()
		},
	})

	store, sess := singleSession()
	res := r.Dispatch("attach-session", []string{"-t", "alpha"}, store, command.ClientView{}, command.NewQueue())
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if gotSess.ID != sess.ID {
		t.Errorf("Target.Session.ID = %q, want %q", gotSess.ID, sess.ID)
	}
}

func TestTargetResolution_BySessionID(t *testing.T) {
	r := command.NewRegistry()
	var gotSess command.SessionView
	mustRegister(t, r, command.Spec{
		Name:   "attach-session",
		Target: command.TargetSpec{Kind: command.TargetSession},
		Run: func(ctx *command.Ctx) command.Result {
			gotSess = ctx.Target.Session
			return command.OK()
		},
	})

	store, sess := singleSession()
	// $s1 — dollar-prefixed session ID
	res := r.Dispatch("attach-session", []string{"-t", "$s1"}, store, command.ClientView{}, command.NewQueue())
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if gotSess.ID != sess.ID {
		t.Errorf("Target.Session.ID = %q, want %q", gotSess.ID, sess.ID)
	}
}

func TestTargetResolution_WindowByIndex(t *testing.T) {
	r := command.NewRegistry()
	var gotWin command.WindowView
	mustRegister(t, r, command.Spec{
		Name:   "select-window",
		Target: command.TargetSpec{Kind: command.TargetWindow},
		Run: func(ctx *command.Ctx) command.Result {
			gotWin = ctx.Target.Window
			return command.OK()
		},
	})

	store, sess := singleSession()
	// alpha:1 — session name + window index
	res := r.Dispatch("select-window", []string{"-t", "alpha:1"}, store, command.ClientView{}, command.NewQueue())
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if gotWin.ID != sess.Windows[0].ID {
		t.Errorf("Target.Window.ID = %q, want %q", gotWin.ID, sess.Windows[0].ID)
	}
}

func TestTargetResolution_WindowByName(t *testing.T) {
	r := command.NewRegistry()
	var gotWin command.WindowView
	mustRegister(t, r, command.Spec{
		Name:   "select-window",
		Target: command.TargetSpec{Kind: command.TargetWindow},
		Run: func(ctx *command.Ctx) command.Result {
			gotWin = ctx.Target.Window
			return command.OK()
		},
	})

	store, sess := singleSession()
	res := r.Dispatch("select-window", []string{"-t", "alpha:main"}, store, command.ClientView{}, command.NewQueue())
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if gotWin.Name != "main" {
		t.Errorf("Target.Window.Name = %q, want %q", gotWin.Name, "main")
	}
	_ = sess
}

func TestTargetResolution_PaneByID(t *testing.T) {
	r := command.NewRegistry()
	var gotPane command.PaneView
	mustRegister(t, r, command.Spec{
		Name:   "select-pane",
		Target: command.TargetSpec{Kind: command.TargetPane},
		Run: func(ctx *command.Ctx) command.Result {
			gotPane = ctx.Target.Pane
			return command.OK()
		},
	})

	store, _ := singleSession()
	// alpha:main.%1 — session + window name + pane %id
	res := r.Dispatch("select-pane", []string{"-t", "alpha:main.%1"}, store, command.ClientView{}, command.NewQueue())
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if gotPane.ID != 1 {
		t.Errorf("Target.Pane.ID = %d, want 1", gotPane.ID)
	}
}

func TestTargetResolution_SessionNotFound(t *testing.T) {
	r := command.NewRegistry()
	mustRegister(t, r, command.Spec{
		Name:   "attach-session",
		Target: command.TargetSpec{Kind: command.TargetSession},
		Run:    func(*command.Ctx) command.Result { return command.OK() },
	})

	store, _ := singleSession()
	res := r.Dispatch("attach-session", []string{"-t", "nonexistent"}, store, command.ClientView{}, command.NewQueue())
	if res.Err == nil {
		t.Error("expected error for missing session, got nil")
	}
}

func TestTargetResolution_NoClientNoDefault(t *testing.T) {
	r := command.NewRegistry()
	mustRegister(t, r, command.Spec{
		Name:   "select-window",
		Target: command.TargetSpec{Kind: command.TargetWindow},
		Run:    func(*command.Ctx) command.Result { return command.OK() },
	})

	store, _ := singleSession()
	// No -t, no attached client → should error.
	res := r.Dispatch("select-window", nil, store, command.ClientView{}, command.NewQueue())
	if res.Err == nil {
		t.Error("expected error when no target and no client, got nil")
	}
}

func TestTargetResolution_TildeLastSession(t *testing.T) {
	r := command.NewRegistry()
	var gotSess command.SessionView
	mustRegister(t, r, command.Spec{
		Name:   "switch-client",
		Target: command.TargetSpec{Kind: command.TargetSession},
		Run: func(ctx *command.Ctx) command.Result {
			gotSess = ctx.Target.Session
			return command.OK()
		},
	})

	pane := command.PaneView{ID: 2, Title: "zsh"}
	win := command.WindowView{ID: "w2", Name: "work", Index: 1, Panes: []command.PaneView{pane}, Active: 2}
	second := command.SessionView{ID: "s2", Name: "beta", Windows: []command.WindowView{win}, Current: 0}
	store, _ := singleSession()
	store.sessions = append(store.sessions, second)

	res := r.Dispatch("switch-client", []string{"-t", "~"}, store, command.ClientView{}, command.NewQueue())
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if gotSess.ID != "s2" {
		t.Errorf("~ resolved to session %q, want %q", gotSess.ID, "s2")
	}
}

// ─── Queue tests ──────────────────────────────────────────────────────────────

func TestQueue_DrainRunsItemsInOrder(t *testing.T) {
	q := command.NewQueue()
	var order []int
	for i := 1; i <= 3; i++ {
		n := i
		q.EnqueueFunc("", func() { order = append(order, n) })
	}
	processed := q.Drain()
	if processed != 3 {
		t.Errorf("Drain() processed %d items, want 3", processed)
	}
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("execution order = %v, want [1 2 3]", order)
	}
}

func TestQueue_DrainEmptyQueue(t *testing.T) {
	q := command.NewQueue()
	if n := q.Drain(); n != 0 {
		t.Errorf("Drain() on empty queue = %d, want 0", n)
	}
}

func TestQueue_CallbackPausesQueue(t *testing.T) {
	q := command.NewQueue()
	var resumed bool
	var resumeFn func()

	q.Enqueue(command.Item{
		Name: "confirm",
		Callback: func(resume func()) {
			resumeFn = resume
		},
	})
	q.EnqueueFunc("after", func() { resumed = true })

	q.Drain() // processes the callback item, then pauses

	if !q.IsPaused() {
		t.Fatal("queue should be paused after Callback item")
	}
	if resumed {
		t.Fatal("item after callback ran before resume")
	}
	if q.Len() != 1 {
		t.Errorf("expected 1 item remaining, got %d", q.Len())
	}

	// Call resume, then drain again.
	resumeFn()
	if q.IsPaused() {
		t.Error("queue still paused after resume()")
	}
	q.Drain()
	if !resumed {
		t.Error("item after callback did not run after resume + drain")
	}
}

func TestQueue_EnqueueFromHandler(t *testing.T) {
	q := command.NewQueue()
	var order []int
	q.EnqueueFunc("first", func() {
		order = append(order, 1)
		q.EnqueueFunc("second", func() { order = append(order, 2) })
	})
	q.Drain()
	// "second" was enqueued by "first"; it should run in the same Drain call.
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Errorf("order = %v, want [1 2]", order)
	}
}

func TestQueue_InjectableLogger(t *testing.T) {
	var buf bytes.Buffer
	q := command.NewQueue()
	q.SetLogger(&buf, "test: ", 0)
	q.EnqueueFunc("myjob", func() {})
	q.Drain()

	if buf.Len() == 0 {
		t.Error("expected log output, got none")
	}
}

func TestQueue_NilLoggerReverts(t *testing.T) {
	q := command.NewQueue()
	q.SetLogger(nil, "", 0) // should not panic
	q.EnqueueFunc("job", func() {})
	q.Drain()               // should not panic or write anything
}

// ─── Result helpers ───────────────────────────────────────────────────────────

func TestResult_OK(t *testing.T) {
	r := command.OK()
	if r.Err != nil {
		t.Errorf("OK().Err = %v, want nil", r.Err)
	}
}

func TestResult_Errorf(t *testing.T) {
	r := command.Errorf("bad %s", "input")
	if r.Err == nil {
		t.Fatal("Errorf().Err = nil")
	}
	if r.Err.Error() != "bad input" {
		t.Errorf("error message = %q, want %q", r.Err.Error(), "bad input")
	}
}

func TestResult_WithOutput(t *testing.T) {
	r := command.WithOutput("hello")
	if r.Output != "hello" {
		t.Errorf("Output = %q, want %q", r.Output, "hello")
	}
	if r.Err != nil {
		t.Errorf("Err = %v, want nil", r.Err)
	}
}

// ─── Dispatch unknown command ─────────────────────────────────────────────────

func TestDispatch_UnknownCommand(t *testing.T) {
	r := command.NewRegistry()
	res := r.Dispatch("no-such-command", nil, &stubServer{}, command.ClientView{}, command.NewQueue())
	if res.Err == nil {
		t.Error("expected error for unknown command, got nil")
	}
	if !errors.Is(res.Err, res.Err) { // just confirm it's non-nil
		t.Error("Err should be non-nil")
	}
}
