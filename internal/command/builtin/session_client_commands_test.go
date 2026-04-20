package builtin_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/command"
)

// ─── has-session tests ────────────────────────────────────────────────────────

func TestHasSession_ExistingSession_ReturnsOK(t *testing.T) {
	b := newBackend()
	res := dispatch("has-session", []string{"-t", "alpha"}, b)
	if res.Err != nil {
		t.Fatalf("has-session for existing session returned error: %v", res.Err)
	}
	if res.Output != "" {
		t.Errorf("has-session should produce no output on success, got: %q", res.Output)
	}
}

func TestHasSession_NonexistentSession_ReturnsError(t *testing.T) {
	b := newBackend()
	res := dispatch("has-session", []string{"-t", "nonexistent"}, b)
	if res.Err == nil {
		t.Fatal("has-session for nonexistent session should return an error")
	}
}

// ─── refresh-client tests ─────────────────────────────────────────────────────

func TestRefreshClient_NoFlags_TriggersRedraw(t *testing.T) {
	b := newBackend()
	res := dispatch("refresh-client", nil, b)
	if res.Err != nil {
		t.Fatalf("refresh-client returned error: %v", res.Err)
	}
	if len(b.refreshedClients) != 1 {
		t.Fatalf("expected 1 RefreshClient call, got %d", len(b.refreshedClients))
	}
	if b.refreshedClients[0] != "c1" {
		t.Errorf("RefreshClient called with %q, want %q", b.refreshedClients[0], "c1")
	}
}

func TestRefreshClient_WithTarget_TriggersRedrawForTarget(t *testing.T) {
	b := newBackend()
	b.clients = append(b.clients, command.ClientView{ID: "c2", SessionID: "s1", KeyTable: "root"})
	res := dispatch("refresh-client", []string{"-t", "c2"}, b)
	if res.Err != nil {
		t.Fatalf("refresh-client -t c2 returned error: %v", res.Err)
	}
	if len(b.refreshedClients) != 1 || b.refreshedClients[0] != "c2" {
		t.Errorf("RefreshClient called with %v, want [c2]", b.refreshedClients)
	}
}

func TestRefreshClient_DetachFlag_DetachesClient(t *testing.T) {
	b := newBackend()
	res := dispatch("refresh-client", []string{"-d"}, b)
	if res.Err != nil {
		t.Fatalf("refresh-client -d returned error: %v", res.Err)
	}
	if len(b.detachedClients) != 1 || b.detachedClients[0] != "c1" {
		t.Errorf("DetachClient called with %v, want [c1]", b.detachedClients)
	}
}

func TestRefreshClient_SizeFlag_ResizesClient(t *testing.T) {
	b := newBackend()
	res := dispatch("refresh-client", []string{"-s", "120x40"}, b)
	if res.Err != nil {
		t.Fatalf("refresh-client -s returned error: %v", res.Err)
	}
	if len(b.resizedClients) != 1 {
		t.Fatalf("expected 1 ResizeClient call, got %d", len(b.resizedClients))
	}
	got := b.resizedClients[0]
	if got.clientID != "c1" || got.cols != 120 || got.rows != 40 {
		t.Errorf("ResizeClient(%q, %d, %d): unexpected args", got.clientID, got.cols, got.rows)
	}
}

func TestRefreshClient_InvalidSize_ReturnsError(t *testing.T) {
	b := newBackend()
	res := dispatch("refresh-client", []string{"-s", "badsize"}, b)
	if res.Err == nil {
		t.Error("refresh-client -s badsize should return an error")
	}
}

// ─── suspend-client tests ─────────────────────────────────────────────────────

func TestSuspendClient_DefaultsToCallingClient(t *testing.T) {
	b := newBackend()
	res := dispatch("suspend-client", nil, b)
	if res.Err != nil {
		t.Fatalf("suspend-client returned error: %v", res.Err)
	}
	if len(b.suspendedClients) != 1 {
		t.Fatalf("expected 1 SuspendClient call, got %d", len(b.suspendedClients))
	}
	if b.suspendedClients[0] != "c1" {
		t.Errorf("SuspendClient called with %q, want %q", b.suspendedClients[0], "c1")
	}
}

func TestSuspendClient_SpecifiedTarget_SuspendsCorrectClient(t *testing.T) {
	b := newBackend()
	b.clients = append(b.clients, command.ClientView{ID: "c2", SessionID: "s1", KeyTable: "root"})
	res := dispatch("suspend-client", []string{"-t", "c2"}, b)
	if res.Err != nil {
		t.Fatalf("suspend-client -t c2 returned error: %v", res.Err)
	}
	if len(b.suspendedClients) != 1 || b.suspendedClients[0] != "c2" {
		t.Errorf("SuspendClient called with %v, want [c2]", b.suspendedClients)
	}
}

// ─── server-access tests ──────────────────────────────────────────────────────

func TestServerAccess_AllowUser_RecordsACLEntry(t *testing.T) {
	b := newBackend()
	res := dispatch("server-access", []string{"-a", "alice"}, b)
	if res.Err != nil {
		t.Fatalf("server-access -a returned error: %v", res.Err)
	}
	if len(b.serverACLEntries) != 1 {
		t.Fatalf("expected 1 ACL entry, got %d", len(b.serverACLEntries))
	}
	e := b.serverACLEntries[0]
	if e.username != "alice" || !e.allow {
		t.Errorf("ACL entry = %+v, want {alice true false}", e)
	}
}

func TestServerAccess_DenyUser_RecordsACLEntry(t *testing.T) {
	b := newBackend()
	res := dispatch("server-access", []string{"-d", "bob"}, b)
	if res.Err != nil {
		t.Fatalf("server-access -d returned error: %v", res.Err)
	}
	if len(b.serverACLEntries) != 1 {
		t.Fatalf("expected 1 ACL entry, got %d", len(b.serverACLEntries))
	}
	e := b.serverACLEntries[0]
	if e.username != "bob" || e.allow {
		t.Errorf("ACL entry = %+v, want {bob false false}", e)
	}
}

func TestServerAccess_DenyAll_SetsDenyAll(t *testing.T) {
	b := newBackend()
	res := dispatch("server-access", []string{"-n"}, b)
	if res.Err != nil {
		t.Fatalf("server-access -n returned error: %v", res.Err)
	}
	if !b.denyAllClientsCalled {
		t.Error("DenyAllClients() was not called")
	}
}

func TestServerAccess_NoFlags_ReturnsError(t *testing.T) {
	b := newBackend()
	res := dispatch("server-access", []string{"alice"}, b)
	if res.Err == nil {
		t.Error("server-access without -a/-d/-n should return an error")
	}
}

func TestServerAccess_AllowWithWrite_RecordsWriteAccess(t *testing.T) {
	b := newBackend()
	res := dispatch("server-access", []string{"-a", "-w", "carol"}, b)
	if res.Err != nil {
		t.Fatalf("server-access -a -w returned error: %v", res.Err)
	}
	if len(b.serverACLEntries) != 1 {
		t.Fatalf("expected 1 ACL entry, got %d", len(b.serverACLEntries))
	}
	e := b.serverACLEntries[0]
	if e.username != "carol" || !e.allow || !e.write {
		t.Errorf("ACL entry = %+v, want {carol true true}", e)
	}
}
