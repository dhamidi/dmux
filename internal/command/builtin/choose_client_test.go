package builtin_test

import (
	"strings"
	"testing"

	"github.com/dhamidi/dmux/internal/command"
)

func TestChooseClient_BuildsItemListFromClients(t *testing.T) {
	b := newBackend()
	b.clients = []command.ClientView{
		{ID: "c1", SessionID: "s1", TTY: "/dev/pts/0"},
		{ID: "c2", SessionID: "s2", TTY: "/dev/pts/1"},
	}

	res := dispatch("choose-client", nil, b)
	if res.Err != nil {
		t.Fatalf("choose-client: unexpected error: %v", res.Err)
	}

	if len(b.chooseClientCalls) != 1 {
		t.Fatalf("expected 1 EnterChooseClient call, got %d", len(b.chooseClientCalls))
	}

	call := b.chooseClientCalls[0]
	if call.template != "switch-client -t '%%'" {
		t.Errorf("default template: want %q, got %q", "switch-client -t '%%'", call.template)
	}
	if len(call.items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(call.items))
	}

	ids := map[string]bool{}
	for _, item := range call.items {
		ids[item.Value] = true
		if !strings.Contains(item.Display, item.Value) {
			t.Errorf("item display %q does not contain client ID %q", item.Display, item.Value)
		}
	}
	if !ids["c1"] || !ids["c2"] {
		t.Errorf("expected items for c1 and c2, got values: %v", ids)
	}
}

func TestChooseClient_CustomTemplate(t *testing.T) {
	b := newBackend()
	b.clients = []command.ClientView{{ID: "c1", SessionID: "s1"}}

	res := dispatch("choose-client", []string{"detach-client -t '%%'"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}

	if len(b.chooseClientCalls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(b.chooseClientCalls))
	}
	if got := b.chooseClientCalls[0].template; got != "detach-client -t '%%'" {
		t.Errorf("template: want %q, got %q", "detach-client -t '%%'", got)
	}
}

func TestChooseClient_DetachedClientShownAsDetached(t *testing.T) {
	b := newBackend()
	b.clients = []command.ClientView{
		{ID: "c3", SessionID: "", TTY: "/dev/pts/2"},
	}

	dispatch("choose-client", nil, b)

	if len(b.chooseClientCalls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(b.chooseClientCalls))
	}
	items := b.chooseClientCalls[0].items
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if !strings.Contains(items[0].Display, "detached") {
		t.Errorf("detached client display should contain 'detached', got %q", items[0].Display)
	}
}

func TestChooseClient_TTYShownInDisplay(t *testing.T) {
	b := newBackend()
	b.clients = []command.ClientView{
		{ID: "c1", SessionID: "s1", TTY: "/dev/pts/99"},
	}

	dispatch("choose-client", nil, b)

	if len(b.chooseClientCalls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(b.chooseClientCalls))
	}
	items := b.chooseClientCalls[0].items
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if !strings.Contains(items[0].Display, "/dev/pts/99") {
		t.Errorf("display should contain TTY '/dev/pts/99', got %q", items[0].Display)
	}
}
