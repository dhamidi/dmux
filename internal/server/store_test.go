package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/session"
)

// mockPane satisfies session.Pane without real I/O.
type mockPane struct{ title string }

func (p *mockPane) Title() string                          { return p.title }
func (p *mockPane) Resize(_, _ int) error                 { return nil }
func (p *mockPane) Close() error                          { return nil }
func (p *mockPane) CaptureContent(_ bool) ([]byte, error) { return nil, nil }
func (p *mockPane) Respawn(_ string) error                { return nil }
func (p *mockPane) SendKey(_ keys.Key) error              { return nil }
func (p *mockPane) Write(_ []byte) error                  { return nil }
func (p *mockPane) Snapshot() pane.CellGrid               { return pane.CellGrid{} }

// helpers

func newState() *session.Server { return session.NewServer() }

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

// ─── GetSession ────────────────────────────────────────────────────────────────

func TestServerStore_GetSession_Empty(t *testing.T) {
	st := newServerStore(newState())
	if _, ok := st.GetSession("s1"); ok {
		t.Fatal("expected false for unknown session")
	}
}

func TestServerStore_GetSession_Found(t *testing.T) {
	srv := newState()
	addSession(srv, "s1", "main")
	st := newServerStore(srv)

	v, ok := st.GetSession("s1")
	if !ok {
		t.Fatal("expected true for existing session")
	}
	if v.ID != "s1" || v.Name != "main" {
		t.Fatalf("got {%q %q}, want {s1 main}", v.ID, v.Name)
	}
}

func TestServerStore_GetSession_Unknown(t *testing.T) {
	srv := newState()
	addSession(srv, "s1", "main")
	st := newServerStore(srv)

	if _, ok := st.GetSession("nonexistent"); ok {
		t.Fatal("expected false for unknown ID")
	}
}

func TestServerStore_GetSession_CurrentWindow(t *testing.T) {
	srv := newState()
	s := addSession(srv, "s1", "main")
	addWindow(s, "w1", "bash")
	addWindow(s, "w2", "vim")
	// Current defaults to first window (index 0 in slice).
	st := newServerStore(srv)

	v, _ := st.GetSession("s1")
	if v.Current != 0 {
		t.Fatalf("Current = %d, want 0", v.Current)
	}
	if len(v.Windows) != 2 {
		t.Fatalf("len(Windows) = %d, want 2", len(v.Windows))
	}
}

func TestServerStore_GetSession_NoCurrent(t *testing.T) {
	srv := newState()
	s := addSession(srv, "s1", "main")
	// Leave Current nil (no windows added).
	_ = s
	st := newServerStore(srv)

	v, _ := st.GetSession("s1")
	if v.Current != -1 {
		t.Fatalf("Current = %d, want -1 when no current window", v.Current)
	}
}

// ─── GetSessionByName ─────────────────────────────────────────────────────────

func TestServerStore_GetSessionByName_Empty(t *testing.T) {
	st := newServerStore(newState())
	if _, ok := st.GetSessionByName("main"); ok {
		t.Fatal("expected false on empty state")
	}
}

func TestServerStore_GetSessionByName_Found(t *testing.T) {
	srv := newState()
	addSession(srv, "s1", "main")
	st := newServerStore(srv)

	v, ok := st.GetSessionByName("main")
	if !ok {
		t.Fatal("expected true")
	}
	if v.Name != "main" {
		t.Fatalf("Name = %q, want main", v.Name)
	}
}

func TestServerStore_GetSessionByName_NotFound(t *testing.T) {
	srv := newState()
	addSession(srv, "s1", "main")
	st := newServerStore(srv)

	if _, ok := st.GetSessionByName("other"); ok {
		t.Fatal("expected false for non-matching name")
	}
}

func TestServerStore_GetSessionByName_DuplicateNames(t *testing.T) {
	// When two sessions share a name, GetSessionByName must return one of them
	// (the spec says "first match"; map iteration order is non-deterministic, so
	// we just assert that a result is returned and has the right name).
	srv := newState()
	addSession(srv, "s1", "dup")
	addSession(srv, "s2", "dup")
	st := newServerStore(srv)

	v, ok := st.GetSessionByName("dup")
	if !ok {
		t.Fatal("expected true for duplicated name")
	}
	if v.Name != "dup" {
		t.Fatalf("Name = %q, want dup", v.Name)
	}
}

// ─── ListSessions ─────────────────────────────────────────────────────────────

func TestServerStore_ListSessions_Empty(t *testing.T) {
	st := newServerStore(newState())
	if got := st.ListSessions(); len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestServerStore_ListSessions_One(t *testing.T) {
	srv := newState()
	addSession(srv, "s1", "main")
	st := newServerStore(srv)

	got := st.ListSessions()
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].ID != "s1" {
		t.Fatalf("ID = %q, want s1", got[0].ID)
	}
}

// ─── GetClient ────────────────────────────────────────────────────────────────

func TestServerStore_GetClient_Empty(t *testing.T) {
	st := newServerStore(newState())
	if _, ok := st.GetClient("c1"); ok {
		t.Fatal("expected false on empty state")
	}
}

func TestServerStore_GetClient_Found(t *testing.T) {
	srv := newState()
	addSession(srv, "s1", "main")
	c := session.NewClient("c1")
	c.TTY = "/dev/pts/1"
	c.Size = session.Size{Cols: 80, Rows: 24}
	_ = srv.AttachClient(c, "s1")
	st := newServerStore(srv)

	v, ok := st.GetClient("c1")
	if !ok {
		t.Fatal("expected true")
	}
	if v.ID != "c1" {
		t.Fatalf("ID = %q, want c1", v.ID)
	}
	if v.SessionID != "s1" {
		t.Fatalf("SessionID = %q, want s1", v.SessionID)
	}
	if v.Cols != 80 || v.Rows != 24 {
		t.Fatalf("Size = %dx%d, want 80x24", v.Cols, v.Rows)
	}
	if v.TTY != "/dev/pts/1" {
		t.Fatalf("TTY = %q, want /dev/pts/1", v.TTY)
	}
}

func TestServerStore_GetClient_DetachedSession(t *testing.T) {
	srv := newState()
	c := session.NewClient("c1")
	// Register client without attaching to a session.
	srv.Clients[session.ClientID("c1")] = c
	st := newServerStore(srv)

	v, ok := st.GetClient("c1")
	if !ok {
		t.Fatal("expected true")
	}
	if v.SessionID != "" {
		t.Fatalf("SessionID = %q, want empty for detached client", v.SessionID)
	}
}

func TestServerStore_GetClient_Unknown(t *testing.T) {
	srv := newState()
	addSession(srv, "s1", "main")
	c := session.NewClient("c1")
	_ = srv.AttachClient(c, "s1")
	st := newServerStore(srv)

	if _, ok := st.GetClient("nonexistent"); ok {
		t.Fatal("expected false for unknown ID")
	}
}

// ─── ListClients ──────────────────────────────────────────────────────────────

func TestServerStore_ListClients_Empty(t *testing.T) {
	st := newServerStore(newState())
	if got := st.ListClients(); len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestServerStore_ListClients_One(t *testing.T) {
	srv := newState()
	addSession(srv, "s1", "main")
	c := session.NewClient("c1")
	_ = srv.AttachClient(c, "s1")
	st := newServerStore(srv)

	got := st.ListClients()
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].ID != "c1" {
		t.Fatalf("ID = %q, want c1", got[0].ID)
	}
}

// ─── WindowView / PaneView conversion ────────────────────────────────────────

func TestServerStore_WindowViewFields(t *testing.T) {
	srv := newState()
	s := addSession(srv, "s1", "main")
	w, wl := addWindow(s, "w1", "bash")
	w.AddPane(1, &mockPane{title: "shell"})
	w.AddPane(2, &mockPane{title: "vim"})
	_ = wl
	st := newServerStore(srv)

	v, _ := st.GetSession("s1")
	if len(v.Windows) != 1 {
		t.Fatalf("len(Windows) = %d, want 1", len(v.Windows))
	}
	wv := v.Windows[0]
	if wv.ID != "w1" || wv.Name != "bash" {
		t.Fatalf("Window {%q %q}, want {w1 bash}", wv.ID, wv.Name)
	}
	if wv.Index != 1 {
		t.Fatalf("Index = %d, want 1", wv.Index)
	}
	if len(wv.Panes) != 2 {
		t.Fatalf("len(Panes) = %d, want 2", len(wv.Panes))
	}
	if wv.Active != 1 {
		t.Fatalf("Active = %d, want 1", wv.Active)
	}
}
