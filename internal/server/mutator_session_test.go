package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/session"
)

// newTestMutator creates a serverMutator backed by a fresh session.Server.
// The returned shutdown channel is closed when KillServer is called.
func newTestMutator() (*serverMutator, chan struct{}) {
	state := session.NewServer()
	done := make(chan struct{})
	m := &serverMutator{
		state:    state,
		shutdown: func() { close(done) },
	}
	return m, done
}

func TestNewSession(t *testing.T) {
	m, _ := newTestMutator()

	v1, err := m.NewSession("alpha")
	if err != nil {
		t.Fatalf("NewSession #1: %v", err)
	}
	v2, err := m.NewSession("beta")
	if err != nil {
		t.Fatalf("NewSession #2: %v", err)
	}

	if got := len(m.state.Sessions); got != 2 {
		t.Errorf("Sessions length = %d, want 2", got)
	}
	if v1.ID == v2.ID {
		t.Errorf("IDs are not distinct: %q", v1.ID)
	}
	if v1.Name != "alpha" {
		t.Errorf("v1.Name = %q, want %q", v1.Name, "alpha")
	}
	if v2.Name != "beta" {
		t.Errorf("v2.Name = %q, want %q", v2.Name, "beta")
	}
}

func TestNewSessionDefaultName(t *testing.T) {
	m, _ := newTestMutator()

	v, err := m.NewSession("")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if v.Name == "" {
		t.Error("expected non-empty auto-generated name, got empty string")
	}
}

func TestKillSession(t *testing.T) {
	m, _ := newTestMutator()

	v, err := m.NewSession("to-kill")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if err := m.KillSession(v.ID); err != nil {
		t.Fatalf("KillSession: %v", err)
	}
	if _, ok := m.state.Sessions[session.SessionID(v.ID)]; ok {
		t.Errorf("session %q still present after KillSession", v.ID)
	}
}

func TestKillSessionNotFound(t *testing.T) {
	m, _ := newTestMutator()

	if err := m.KillSession("nonexistent"); err == nil {
		t.Error("expected error for unknown session ID, got nil")
	}
}

func TestRenameSession(t *testing.T) {
	m, _ := newTestMutator()

	v, err := m.NewSession("original")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if err := m.RenameSession(v.ID, "renamed"); err != nil {
		t.Fatalf("RenameSession: %v", err)
	}
	sess := m.state.Sessions[session.SessionID(v.ID)]
	if sess.Name != "renamed" {
		t.Errorf("sess.Name = %q, want %q", sess.Name, "renamed")
	}
}

func TestRenameSessionNotFound(t *testing.T) {
	m, _ := newTestMutator()

	if err := m.RenameSession("nonexistent", "newname"); err == nil {
		t.Error("expected error for unknown session ID, got nil")
	}
}

func TestKillServer(t *testing.T) {
	m, done := newTestMutator()

	if err := m.KillServer(); err != nil {
		t.Fatalf("KillServer: %v", err)
	}

	select {
	case <-done:
		// shutdown was invoked
	default:
		t.Error("KillServer did not invoke the shutdown function")
	}
}
