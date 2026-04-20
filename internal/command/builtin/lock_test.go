package builtin_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/command"
)

// newBackendTwoClients returns a backend with two clients attached to
// different sessions, for use in lock-client / lock-session tests.
func newBackendTwoClients() *testBackend {
	b := newBackend()
	// Add a second session and a second client attached to it.
	sess2 := command.SessionView{
		ID: "s2", Name: "beta",
		Windows: []command.WindowView{
			{ID: "w3", Name: "main", Index: 0, Panes: []command.PaneView{{ID: 3, Title: "bash"}}, Active: 3},
		},
		Current: 0,
	}
	b.sessions = append(b.sessions, sess2)
	b.clients = append(b.clients, command.ClientView{ID: "c2", SessionID: "s2", KeyTable: "root"})
	return b
}

// ─── lock-client tests ────────────────────────────────────────────────────────

func TestLockClient_DefaultsToCallingClient(t *testing.T) {
	b := newBackendTwoClients()
	res := dispatch("lock-client", nil, b)
	if res.Err != nil {
		t.Fatalf("lock-client returned error: %v", res.Err)
	}
	if len(b.lockedClients) != 1 {
		t.Fatalf("expected 1 LockClient call, got %d: %v", len(b.lockedClients), b.lockedClients)
	}
	if b.lockedClients[0] != "c1" {
		t.Errorf("LockClient called with %q, want %q", b.lockedClients[0], "c1")
	}
}

func TestLockClient_LocksSpecifiedClient(t *testing.T) {
	b := newBackendTwoClients()
	res := dispatch("lock-client", []string{"-t", "c2"}, b)
	if res.Err != nil {
		t.Fatalf("lock-client -t c2 returned error: %v", res.Err)
	}
	if len(b.lockedClients) != 1 {
		t.Fatalf("expected 1 LockClient call, got %d: %v", len(b.lockedClients), b.lockedClients)
	}
	if b.lockedClients[0] != "c2" {
		t.Errorf("LockClient called with %q, want %q", b.lockedClients[0], "c2")
	}
}

func TestLockClient_DoesNotLockOtherClients(t *testing.T) {
	b := newBackendTwoClients()
	res := dispatch("lock-client", []string{"-t", "c1"}, b)
	if res.Err != nil {
		t.Fatalf("lock-client -t c1 returned error: %v", res.Err)
	}
	for _, id := range b.lockedClients {
		if id == "c2" {
			t.Errorf("lock-client locked c2 but only c1 was targeted")
		}
	}
}

// ─── lock-session tests ───────────────────────────────────────────────────────

func TestLockSession_LocksAllClientsOfSession(t *testing.T) {
	b := newBackend()
	// Add a second client attached to the same session s1.
	b.clients = append(b.clients, command.ClientView{ID: "c2", SessionID: "s1", KeyTable: "root"})

	res := dispatch("lock-session", []string{"-t", "alpha"}, b)
	if res.Err != nil {
		t.Fatalf("lock-session returned error: %v", res.Err)
	}
	if len(b.lockedClients) != 2 {
		t.Fatalf("expected 2 LockClient calls, got %d: %v", len(b.lockedClients), b.lockedClients)
	}
	locked := map[string]bool{}
	for _, id := range b.lockedClients {
		locked[id] = true
	}
	if !locked["c1"] || !locked["c2"] {
		t.Errorf("expected both c1 and c2 to be locked, got: %v", b.lockedClients)
	}
}

func TestLockSession_DoesNotLockClientsOfOtherSessions(t *testing.T) {
	b := newBackendTwoClients()
	// Lock session "alpha" (s1) — only c1 should be locked, not c2 (which is in s2/beta).
	res := dispatch("lock-session", []string{"-t", "alpha"}, b)
	if res.Err != nil {
		t.Fatalf("lock-session returned error: %v", res.Err)
	}
	for _, id := range b.lockedClients {
		if id == "c2" {
			t.Errorf("lock-session locked c2 which belongs to a different session")
		}
	}
	if len(b.lockedClients) != 1 || b.lockedClients[0] != "c1" {
		t.Errorf("expected only c1 to be locked, got: %v", b.lockedClients)
	}
}
