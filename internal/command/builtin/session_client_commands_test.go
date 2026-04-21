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

func TestRefreshClient_FeatureFlag_SetsFeatures(t *testing.T) {
	b := newBackend()
	res := dispatch("refresh-client", []string{"-f", "256,RGB"}, b)
	if res.Err != nil {
		t.Fatalf("refresh-client -f returned error: %v", res.Err)
	}
	if len(b.setClientFeaturesCalls) != 1 {
		t.Fatalf("expected 1 SetClientFeatures call, got %d", len(b.setClientFeaturesCalls))
	}
	got := b.setClientFeaturesCalls[0]
	if got.clientID != "c1" {
		t.Errorf("SetClientFeatures clientID = %q, want %q", got.clientID, "c1")
	}
	if got.features != "256,RGB" {
		t.Errorf("SetClientFeatures features = %q, want %q", got.features, "256,RGB")
	}
}

func TestRefreshClient_ClipboardFlag_RequestsClipboard(t *testing.T) {
	b := newBackend()
	res := dispatch("refresh-client", []string{"-l"}, b)
	if res.Err != nil {
		t.Fatalf("refresh-client -l returned error: %v", res.Err)
	}
	if len(b.requestClipboardCalls) != 1 {
		t.Fatalf("expected 1 RequestClientClipboard call, got %d", len(b.requestClipboardCalls))
	}
	if b.requestClipboardCalls[0] != "c1" {
		t.Errorf("RequestClientClipboard clientID = %q, want %q", b.requestClipboardCalls[0], "c1")
	}
}

func TestRefreshClient_BFlag_RegistersSubscription(t *testing.T) {
	b := newBackend()
	res := dispatch("refresh-client", []string{"-B", "myalert:bell:Bell!"}, b)
	if res.Err != nil {
		t.Fatalf("refresh-client -B returned error: %v", res.Err)
	}
	if len(b.addSubscriptionCalls) != 1 {
		t.Fatalf("expected 1 AddClientSubscription call, got %d", len(b.addSubscriptionCalls))
	}
	got := b.addSubscriptionCalls[0]
	if got.clientID != "c1" || got.name != "myalert" || got.notify != "bell" || got.format != "Bell!" {
		t.Errorf("AddClientSubscription = %+v, want {c1, myalert, bell, Bell!}", got)
	}
}

func TestRefreshClient_BFlag_InvalidFormat_ReturnsError(t *testing.T) {
	b := newBackend()
	res := dispatch("refresh-client", []string{"-B", "no-colon"}, b)
	if res.Err == nil {
		t.Error("refresh-client -B with invalid value should return an error")
	}
}

func TestRefreshClient_BFlag_ColonInFormat_ParsesCorrectly(t *testing.T) {
	b := newBackend()
	// Format field itself contains colons — only the first two are separators.
	res := dispatch("refresh-client", []string{"-B", "a:b:fmt:with:colons"}, b)
	if res.Err != nil {
		t.Fatalf("refresh-client -B returned error: %v", res.Err)
	}
	if len(b.addSubscriptionCalls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(b.addSubscriptionCalls))
	}
	if b.addSubscriptionCalls[0].format != "fmt:with:colons" {
		t.Errorf("format = %q, want %q", b.addSubscriptionCalls[0].format, "fmt:with:colons")
	}
}

func TestRefreshClient_ScrollDown_CallsScrollViewport(t *testing.T) {
	b := newBackend()
	res := dispatch("refresh-client", []string{"-D"}, b)
	if res.Err != nil {
		t.Fatalf("refresh-client -D returned error: %v", res.Err)
	}
	if len(b.scrollViewportCalls) != 1 {
		t.Fatalf("expected 1 ScrollClientViewport call, got %d", len(b.scrollViewportCalls))
	}
	got := b.scrollViewportCalls[0]
	if got.dx != 0 || got.dy != 1 {
		t.Errorf("ScrollClientViewport(%d, %d), want (0, 1)", got.dx, got.dy)
	}
}

func TestRefreshClient_ScrollUp_CallsScrollViewport(t *testing.T) {
	b := newBackend()
	res := dispatch("refresh-client", []string{"-U"}, b)
	if res.Err != nil {
		t.Fatalf("refresh-client -U returned error: %v", res.Err)
	}
	if len(b.scrollViewportCalls) != 1 {
		t.Fatalf("expected 1 ScrollClientViewport call, got %d", len(b.scrollViewportCalls))
	}
	got := b.scrollViewportCalls[0]
	if got.dx != 0 || got.dy != -1 {
		t.Errorf("ScrollClientViewport(%d, %d), want (0, -1)", got.dx, got.dy)
	}
}

func TestRefreshClient_ScrollLeft_CallsScrollViewport(t *testing.T) {
	b := newBackend()
	res := dispatch("refresh-client", []string{"-L"}, b)
	if res.Err != nil {
		t.Fatalf("refresh-client -L returned error: %v", res.Err)
	}
	if len(b.scrollViewportCalls) != 1 {
		t.Fatalf("expected 1 ScrollClientViewport call, got %d", len(b.scrollViewportCalls))
	}
	got := b.scrollViewportCalls[0]
	if got.dx != -1 || got.dy != 0 {
		t.Errorf("ScrollClientViewport(%d, %d), want (-1, 0)", got.dx, got.dy)
	}
}

func TestRefreshClient_ScrollRight_CallsScrollViewport(t *testing.T) {
	b := newBackend()
	res := dispatch("refresh-client", []string{"-R"}, b)
	if res.Err != nil {
		t.Fatalf("refresh-client -R returned error: %v", res.Err)
	}
	if len(b.scrollViewportCalls) != 1 {
		t.Fatalf("expected 1 ScrollClientViewport call, got %d", len(b.scrollViewportCalls))
	}
	got := b.scrollViewportCalls[0]
	if got.dx != 1 || got.dy != 0 {
		t.Errorf("ScrollClientViewport(%d, %d), want (1, 0)", got.dx, got.dy)
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
