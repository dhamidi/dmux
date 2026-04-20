package session_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/session"
)

// ---------- test doubles ----------

// mockPane satisfies session.Pane without any real I/O.
type mockPane struct {
	title string
	cols  int
	rows  int
}

func (p *mockPane) Title() string      { return p.title }
func (p *mockPane) Resize(c, r int)   { p.cols = c; p.rows = r }
func (p *mockPane) Close() error       { return nil }

// mockOverlay satisfies session.Overlay.
type mockOverlay struct{ name string }

func (o *mockOverlay) OverlayName() string { return o.name }

// ---------- helpers ----------

func newServer(t *testing.T) *session.Server {
	t.Helper()
	return session.NewServer()
}

func addSession(srv *session.Server, id, name string) *session.Session {
	s := session.NewSession(session.SessionID(id), name, srv.Options)
	srv.AddSession(s)
	return s
}

func addWindow(s *session.Session, id, name string) (*session.Window, *session.Winlink) {
	w := session.NewWindow(session.WindowID(id), name, s.Options)
	wl := s.AddWindow(w)
	return w, wl
}

// ---------- Server tests ----------

func TestServer_AddSession(t *testing.T) {
	srv := newServer(t)
	s := addSession(srv, "s1", "main")

	got, ok := srv.Sessions[session.SessionID("s1")]
	if !ok {
		t.Fatal("session not found after AddSession")
	}
	if got != s {
		t.Fatalf("stored session pointer mismatch")
	}
}

func TestServer_RemoveSession(t *testing.T) {
	srv := newServer(t)
	addSession(srv, "s1", "main")
	srv.RemoveSession("s1")

	if _, ok := srv.Sessions["s1"]; ok {
		t.Fatal("session still present after RemoveSession")
	}
}

func TestServer_RemoveSession_DetachesClients(t *testing.T) {
	srv := newServer(t)
	addSession(srv, "s1", "main")

	c := session.NewClient("c1")
	if err := srv.AttachClient(c, "s1"); err != nil {
		t.Fatalf("AttachClient: %v", err)
	}

	srv.RemoveSession("s1")

	if c.Session != nil {
		t.Fatal("client still attached after session removed")
	}
}

func TestServer_AttachClient(t *testing.T) {
	srv := newServer(t)
	s := addSession(srv, "s1", "main")

	c := session.NewClient("c1")
	if err := srv.AttachClient(c, "s1"); err != nil {
		t.Fatalf("AttachClient: %v", err)
	}

	if c.Session != s {
		t.Fatalf("client.Session = %v, want %v", c.Session, s)
	}
	if _, ok := srv.Clients["c1"]; !ok {
		t.Fatal("client not registered in server after AttachClient")
	}
}

func TestServer_AttachClient_UnknownSession(t *testing.T) {
	srv := newServer(t)
	c := session.NewClient("c1")
	if err := srv.AttachClient(c, "nonexistent"); err == nil {
		t.Fatal("expected error when attaching to nonexistent session")
	}
}

func TestServer_DetachClient(t *testing.T) {
	srv := newServer(t)
	addSession(srv, "s1", "main")

	c := session.NewClient("c1")
	_ = srv.AttachClient(c, "s1")
	srv.DetachClient("c1")

	if c.Session != nil {
		t.Fatal("client.Session should be nil after DetachClient")
	}
	if _, ok := srv.Clients["c1"]; ok {
		t.Fatal("client still in server.Clients after DetachClient")
	}
}

func TestServer_DetachClient_Noop(t *testing.T) {
	srv := newServer(t)
	// Should not panic when detaching an unknown client.
	srv.DetachClient("nonexistent")
}

// ---------- Session tests ----------

func TestSession_AddWindow(t *testing.T) {
	srv := newServer(t)
	s := addSession(srv, "s1", "main")

	w, wl := addWindow(s, "w1", "bash")

	if len(s.Windows) != 1 {
		t.Fatalf("len(Windows) = %d, want 1", len(s.Windows))
	}
	if wl.Index != 1 {
		t.Fatalf("Winlink.Index = %d, want 1", wl.Index)
	}
	if wl.Window != w {
		t.Fatalf("Winlink.Window pointer mismatch")
	}
	if s.Current != wl {
		t.Fatalf("first window should become Current")
	}
}

func TestSession_AddWindow_IncrementingIndex(t *testing.T) {
	srv := newServer(t)
	s := addSession(srv, "s1", "main")

	_, wl1 := addWindow(s, "w1", "bash")
	_, wl2 := addWindow(s, "w2", "vim")
	_, wl3 := addWindow(s, "w3", "htop")

	if wl1.Index != 1 || wl2.Index != 2 || wl3.Index != 3 {
		t.Fatalf("indices = %d %d %d, want 1 2 3", wl1.Index, wl2.Index, wl3.Index)
	}
}

func TestSession_AddWindow_CurrentNotChangedOnSubsequent(t *testing.T) {
	srv := newServer(t)
	s := addSession(srv, "s1", "main")

	_, wl1 := addWindow(s, "w1", "bash")
	_, _ = addWindow(s, "w2", "vim")

	if s.Current != wl1 {
		t.Fatalf("Current should stay on first window after adding second")
	}
}

func TestSession_RemoveWindow_Middle(t *testing.T) {
	srv := newServer(t)
	s := addSession(srv, "s1", "main")

	addWindow(s, "w1", "bash")
	_, wl2 := addWindow(s, "w2", "vim")
	_, wl3 := addWindow(s, "w3", "htop")

	// Make wl2 current, then remove it — Current should advance to wl3.
	s.Current = wl2
	s.RemoveWindow(1) // index 1 = wl2

	if len(s.Windows) != 2 {
		t.Fatalf("len(Windows) = %d, want 2", len(s.Windows))
	}
	if s.Current != wl3 {
		t.Fatalf("Current should advance to next window after removal")
	}
}

func TestSession_RemoveWindow_Last(t *testing.T) {
	srv := newServer(t)
	s := addSession(srv, "s1", "main")

	_, wl1 := addWindow(s, "w1", "bash")
	_, wl2 := addWindow(s, "w2", "vim")

	s.Current = wl2
	s.RemoveWindow(1) // remove last

	if s.Current != wl1 {
		t.Fatalf("Current should retreat to previous window when last is removed")
	}
}

func TestSession_RemoveWindow_Only(t *testing.T) {
	srv := newServer(t)
	s := addSession(srv, "s1", "main")

	_, _ = addWindow(s, "w1", "bash")
	s.RemoveWindow(0)

	if len(s.Windows) != 0 {
		t.Fatalf("len(Windows) = %d, want 0", len(s.Windows))
	}
	if s.Current != nil {
		t.Fatalf("Current should be nil when all windows removed")
	}
}

func TestSession_RemoveWindow_OutOfRange(t *testing.T) {
	srv := newServer(t)
	s := addSession(srv, "s1", "main")
	addWindow(s, "w1", "bash")

	// Should not panic.
	s.RemoveWindow(-1)
	s.RemoveWindow(99)
	if len(s.Windows) != 1 {
		t.Fatalf("out-of-range RemoveWindow modified the list")
	}
}

// ---------- Window tests ----------

func TestWindow_AddPane(t *testing.T) {
	srv := newServer(t)
	s := addSession(srv, "s1", "main")
	w, _ := addWindow(s, "w1", "bash")

	p := &mockPane{title: "shell"}
	w.AddPane(1, p)

	if got := w.Panes[1]; got != p {
		t.Fatalf("Panes[1] = %v, want %v", got, p)
	}
	if w.Active != 1 {
		t.Fatalf("Active = %d, want 1", w.Active)
	}
}

func TestWindow_AddPane_ActiveNotChangedOnSubsequent(t *testing.T) {
	srv := newServer(t)
	s := addSession(srv, "s1", "main")
	w, _ := addWindow(s, "w1", "bash")

	p1 := &mockPane{title: "shell"}
	p2 := &mockPane{title: "vim"}
	w.AddPane(1, p1)
	w.AddPane(2, p2)

	if w.Active != 1 {
		t.Fatalf("Active should stay on first pane after adding second")
	}
}

func TestWindow_RemovePane(t *testing.T) {
	srv := newServer(t)
	s := addSession(srv, "s1", "main")
	w, _ := addWindow(s, "w1", "bash")

	p1 := &mockPane{title: "shell"}
	p2 := &mockPane{title: "vim"}
	w.AddPane(1, p1)
	w.AddPane(2, p2)

	w.RemovePane(1)

	if _, ok := w.Panes[1]; ok {
		t.Fatal("pane still present after RemovePane")
	}
	if w.Active != 0 {
		t.Fatalf("Active should reset to 0 when active pane is removed")
	}
}

func TestWindow_RemovePane_NonActive(t *testing.T) {
	srv := newServer(t)
	s := addSession(srv, "s1", "main")
	w, _ := addWindow(s, "w1", "bash")

	p1 := &mockPane{title: "shell"}
	p2 := &mockPane{title: "vim"}
	w.AddPane(1, p1)
	w.AddPane(2, p2)

	w.RemovePane(2) // remove non-active pane

	if w.Active != 1 {
		t.Fatalf("Active should stay 1 when non-active pane removed")
	}
}

// ---------- Client tests ----------

func TestClient_Defaults(t *testing.T) {
	c := session.NewClient("c1")
	if c.KeyTable != "root" {
		t.Fatalf("KeyTable = %q, want %q", c.KeyTable, "root")
	}
	if c.Session != nil {
		t.Fatal("Session should be nil before AttachClient")
	}
}

func TestClient_PushPopOverlay(t *testing.T) {
	c := session.NewClient("c1")

	if got := c.PopOverlay(); got != nil {
		t.Fatalf("PopOverlay on empty stack = %v, want nil", got)
	}

	o1 := &mockOverlay{"copy-mode"}
	o2 := &mockOverlay{"tree-mode"}
	c.PushOverlay(o1)
	c.PushOverlay(o2)

	if got := c.PopOverlay(); got != o2 {
		t.Fatalf("PopOverlay = %v, want %v", got, o2)
	}
	if got := c.PopOverlay(); got != o1 {
		t.Fatalf("PopOverlay = %v, want %v", got, o1)
	}
	if got := c.PopOverlay(); got != nil {
		t.Fatalf("PopOverlay on empty stack = %v, want nil", got)
	}
}

// ---------- BufferStack tests ----------

func TestBufferStack_PushPop(t *testing.T) {
	bs := &session.BufferStack{}

	if bs.Top() != nil {
		t.Fatal("Top on empty stack should be nil")
	}
	if bs.Pop() != nil {
		t.Fatal("Pop on empty stack should be nil")
	}

	bs.Push("buf0", []byte("hello"))
	bs.Push("buf1", []byte("world"))

	if bs.Len() != 2 {
		t.Fatalf("Len = %d, want 2", bs.Len())
	}
	if top := bs.Top(); top.Name != "buf1" {
		t.Fatalf("Top.Name = %q, want %q", top.Name, "buf1")
	}

	got := bs.Pop()
	if got.Name != "buf1" || string(got.Data) != "world" {
		t.Fatalf("Pop = {%q %q}, want {buf1 world}", got.Name, got.Data)
	}
	if bs.Len() != 1 {
		t.Fatalf("Len after Pop = %d, want 1", bs.Len())
	}
}

func TestBufferStack_Get(t *testing.T) {
	bs := &session.BufferStack{}
	bs.Push("a", []byte("A"))
	bs.Push("b", []byte("B"))

	// After two pushes: index 0 = "b" (newest), index 1 = "a".
	if got := bs.Get(0); got.Name != "b" {
		t.Fatalf("Get(0).Name = %q, want %q", got.Name, "b")
	}
	if got := bs.Get(1); got.Name != "a" {
		t.Fatalf("Get(1).Name = %q, want %q", got.Name, "a")
	}
	if got := bs.Get(2); got != nil {
		t.Fatalf("Get(2) = %v, want nil", got)
	}
}

func TestBufferStack_Delete(t *testing.T) {
	bs := &session.BufferStack{}
	bs.Push("a", []byte("A"))
	bs.Push("b", []byte("B"))
	bs.Push("c", []byte("C"))
	// Stack (top-first): c, b, a

	bs.Delete(1) // remove "b"
	if bs.Len() != 2 {
		t.Fatalf("Len after Delete = %d, want 2", bs.Len())
	}
	if bs.Get(0).Name != "c" || bs.Get(1).Name != "a" {
		t.Fatalf("after Delete(1): got [%s %s], want [c a]",
			bs.Get(0).Name, bs.Get(1).Name)
	}

	// Out-of-range deletes are no-ops.
	bs.Delete(-1)
	bs.Delete(99)
	if bs.Len() != 2 {
		t.Fatal("out-of-range Delete changed stack size")
	}
}

func TestBufferStack_PushCopiesData(t *testing.T) {
	bs := &session.BufferStack{}
	data := []byte("original")
	bs.Push("buf", data)
	data[0] = 'X' // mutate source after push

	if got := string(bs.Top().Data); got != "original" {
		t.Fatalf("BufferStack.Push did not copy data; got %q", got)
	}
}

// ---------- HookTable tests ----------

func TestHookTable_RegisterRun(t *testing.T) {
	var ht session.HookTable
	count := 0
	ht.Register("session-created", func() { count++ })
	ht.Register("session-created", func() { count += 10 })

	ht.Run("session-created")
	if count != 11 {
		t.Fatalf("count = %d, want 11", count)
	}

	// Running an unregistered hook is a no-op.
	ht.Run("unknown-event")
}

// ---------- format.Context tests ----------

func TestServer_FormatContext(t *testing.T) {
	srv := newServer(t)
	addSession(srv, "s1", "main")
	addSession(srv, "s2", "work")

	v, ok := srv.Lookup("session_count")
	if !ok || v != "2" {
		t.Fatalf("Lookup(session_count) = (%q, %v), want (\"2\", true)", v, ok)
	}

	children := srv.Children("sessions")
	if len(children) != 2 {
		t.Fatalf("Children(sessions) len = %d, want 2", len(children))
	}
	if srv.Children("unknown") != nil {
		t.Fatal("Children(unknown) should be nil")
	}
}

func TestSession_FormatContext(t *testing.T) {
	srv := newServer(t)
	s := addSession(srv, "s42", "mySession")
	addWindow(s, "w1", "bash")
	addWindow(s, "w2", "vim")

	cases := []struct{ key, want string }{
		{"session_id", "s42"},
		{"session_name", "mySession"},
		{"session_windows", "2"},
	}
	for _, tc := range cases {
		v, ok := s.Lookup(tc.key)
		if !ok || v != tc.want {
			t.Errorf("Lookup(%q) = (%q, %v), want (%q, true)", tc.key, v, ok, tc.want)
		}
	}

	if _, ok := s.Lookup("unknown"); ok {
		t.Error("Lookup(unknown) should return false")
	}

	children := s.Children("windows")
	if len(children) != 2 {
		t.Fatalf("Children(windows) len = %d, want 2", len(children))
	}
}

func TestWinlink_FormatContext(t *testing.T) {
	srv := newServer(t)
	s := addSession(srv, "s1", "main")
	_, wl := addWindow(s, "w7", "editor")

	v, ok := wl.Lookup("window_index")
	if !ok || v != "1" {
		t.Fatalf("Lookup(window_index) = (%q, %v), want (\"1\", true)", v, ok)
	}
	v, ok = wl.Lookup("window_name")
	if !ok || v != "editor" {
		t.Fatalf("Lookup(window_name) = (%q, %v), want (\"editor\", true)", v, ok)
	}
}

func TestWindow_FormatContext(t *testing.T) {
	srv := newServer(t)
	s := addSession(srv, "s1", "main")
	w, _ := addWindow(s, "w3", "terminal")
	w.AddPane(1, &mockPane{title: "shell"})
	w.AddPane(2, &mockPane{title: "vim"})

	cases := []struct{ key, want string }{
		{"window_id", "w3"},
		{"window_name", "terminal"},
		{"window_panes", "2"},
	}
	for _, tc := range cases {
		v, ok := w.Lookup(tc.key)
		if !ok || v != tc.want {
			t.Errorf("Lookup(%q) = (%q, %v), want (%q, true)", tc.key, v, ok, tc.want)
		}
	}
}

func TestClient_FormatContext(t *testing.T) {
	c := session.NewClient("c99")
	c.TTY = "/dev/pts/0"
	c.Term = "xterm-256color"
	c.Size = session.Size{Cols: 220, Rows: 50}
	c.KeyTable = "prefix"
	c.Cwd = "/home/user"

	cases := []struct{ key, want string }{
		{"client_id", "c99"},
		{"client_tty", "/dev/pts/0"},
		{"client_term", "xterm-256color"},
		{"client_width", "220"},
		{"client_height", "50"},
		{"client_key_table", "prefix"},
		{"client_cwd", "/home/user"},
	}
	for _, tc := range cases {
		v, ok := c.Lookup(tc.key)
		if !ok || v != tc.want {
			t.Errorf("Lookup(%q) = (%q, %v), want (%q, true)", tc.key, v, ok, tc.want)
		}
	}

	if c.Children("anything") != nil {
		t.Error("Client.Children should always return nil")
	}
}
