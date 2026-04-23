package session_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/dhamidi/dmux/internal/session"
)

func TestNewSessionMonotonicID(t *testing.T) {
	r := session.NewRegistry()
	a, err := r.NewSession("a")
	if err != nil {
		t.Fatalf("NewSession(a): %v", err)
	}
	b, err := r.NewSession("b")
	if err != nil {
		t.Fatalf("NewSession(b): %v", err)
	}
	c, err := r.NewSession("c")
	if err != nil {
		t.Fatalf("NewSession(c): %v", err)
	}
	if a.ID() != 1 || b.ID() != 2 || c.ID() != 3 {
		t.Fatalf("ids not monotonic from 1: %d %d %d", a.ID(), b.ID(), c.ID())
	}
}

func TestFindSession(t *testing.T) {
	r := session.NewRegistry()
	a, _ := r.NewSession("a")
	if got := r.FindSession(a.ID()); got != a {
		t.Fatalf("FindSession returned different pointer: %p vs %p", got, a)
	}
	if got := r.FindSession(9999); got != nil {
		t.Fatalf("FindSession on missing id returned non-nil: %v", got)
	}
}

func TestFindSessionByName(t *testing.T) {
	r := session.NewRegistry()
	a, _ := r.NewSession("alpha")
	if got := r.FindSessionByName("alpha"); got != a {
		t.Fatalf("FindSessionByName returned different pointer: %p vs %p", got, a)
	}
	if got := r.FindSessionByName("missing"); got != nil {
		t.Fatalf("FindSessionByName on missing name returned non-nil: %v", got)
	}
}

func TestDuplicateSessionName(t *testing.T) {
	r := session.NewRegistry()
	if _, err := r.NewSession("dup"); err != nil {
		t.Fatalf("first NewSession(dup): %v", err)
	}
	_, err := r.NewSession("dup")
	if err == nil {
		t.Fatalf("second NewSession(dup) returned nil error, want ErrDuplicateSession")
	}
	if !errors.Is(err, session.ErrDuplicateSession) {
		t.Fatalf("error does not wrap ErrDuplicateSession: %v", err)
	}
	// Ensure the registry did not grow on the rejected second call.
	if r.Len() != 1 {
		t.Fatalf("Len after duplicate rejected: got %d, want 1", r.Len())
	}
}

func TestSessionsOrderedByID(t *testing.T) {
	r := session.NewRegistry()
	names := []string{"z", "m", "a", "q"}
	for _, n := range names {
		if _, err := r.NewSession(n); err != nil {
			t.Fatalf("NewSession(%q): %v", n, err)
		}
	}
	var ids []session.ID
	for s := range r.Sessions() {
		ids = append(ids, s.ID())
	}
	want := []session.ID{1, 2, 3, 4}
	if !slices.Equal(ids, want) {
		t.Fatalf("Sessions order: got %v, want %v", ids, want)
	}
}

func TestRemoveSession(t *testing.T) {
	r := session.NewRegistry()
	a, _ := r.NewSession("a")
	r.RemoveSession(a.ID())
	if r.Len() != 0 {
		t.Fatalf("Len after remove: got %d, want 0", r.Len())
	}
	if r.FindSession(a.ID()) != nil {
		t.Fatalf("FindSession still returns removed session")
	}
	if r.FindSessionByName("a") != nil {
		t.Fatalf("FindSessionByName still returns removed session")
	}
	// RemoveSession on an unknown id is a no-op.
	r.RemoveSession(9999)
}

func TestSessionAddWindow(t *testing.T) {
	r := session.NewRegistry()
	s, _ := r.NewSession("main")
	if s.CurrentWindow() != nil {
		t.Fatalf("fresh session has non-nil CurrentWindow")
	}
	w, err := s.AddWindow("bash")
	if err != nil {
		t.Fatalf("AddWindow: %v", err)
	}
	if w.Index() != 0 {
		t.Fatalf("window index: got %d, want 0", w.Index())
	}
	if w.Name() != "bash" {
		t.Fatalf("window name: got %q, want %q", w.Name(), "bash")
	}
	if s.CurrentWindow() != w {
		t.Fatalf("CurrentWindow did not return added window")
	}
	// M1 is single-window per session: a second AddWindow errors.
	if _, err := s.AddWindow("second"); err == nil {
		t.Fatalf("second AddWindow returned nil error, want failure")
	}
}

func TestWindowSetActivePane(t *testing.T) {
	r := session.NewRegistry()
	s, _ := r.NewSession("main")
	w, _ := s.AddWindow("bash")
	if w.ActivePane() != nil {
		t.Fatalf("fresh window has non-nil ActivePane")
	}
	// A real *pane.Pane needs a pty and is out of scope for a unit
	// test in the session package; the round-trip with a real pane is
	// exercised by the server's integration path. Here we assert the
	// idempotent-nil contract so accidental mutation in the setter
	// gets caught.
	w.SetActivePane(nil)
	if w.ActivePane() != nil {
		t.Fatalf("SetActivePane(nil) did not result in nil ActivePane")
	}
}
