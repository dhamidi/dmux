package builtin_test

import (
	"strings"
	"testing"
)

func TestChooseBuffer_BuildsItemListFromBuffers(t *testing.T) {
	b := newBackend()
	b.buffers = map[string]string{
		"buf0": "hello world",
		"buf1": "foo",
	}

	res := dispatch("choose-buffer", nil, b)
	if res.Err != nil {
		t.Fatalf("choose-buffer: unexpected error: %v", res.Err)
	}

	if len(b.chooseBufferCalls) != 1 {
		t.Fatalf("expected 1 EnterChooseBuffer call, got %d", len(b.chooseBufferCalls))
	}

	call := b.chooseBufferCalls[0]
	if call.template != "paste-buffer -b '%%'" {
		t.Errorf("default template: want %q, got %q", "paste-buffer -b '%%'", call.template)
	}
	if len(call.items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(call.items))
	}

	// Verify each item has name in display and name as value.
	names := map[string]bool{}
	for _, item := range call.items {
		names[item.Value] = true
		if !strings.Contains(item.Display, item.Value) {
			t.Errorf("item display %q does not contain value %q", item.Display, item.Value)
		}
	}
	if !names["buf0"] || !names["buf1"] {
		t.Errorf("expected items for buf0 and buf1, got values: %v", names)
	}
}

func TestChooseBuffer_CustomTemplate(t *testing.T) {
	b := newBackend()
	b.buffers = map[string]string{"mybuf": "data"}

	res := dispatch("choose-buffer", []string{"load-buffer -b '%%'"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}

	if len(b.chooseBufferCalls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(b.chooseBufferCalls))
	}
	if got := b.chooseBufferCalls[0].template; got != "load-buffer -b '%%'" {
		t.Errorf("template: want %q, got %q", "load-buffer -b '%%'", got)
	}
}

func TestChooseBuffer_SizeShownInDisplay(t *testing.T) {
	b := newBackend()
	b.buffers = map[string]string{"abuf": "hello"} // 5 bytes

	dispatch("choose-buffer", nil, b)

	if len(b.chooseBufferCalls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(b.chooseBufferCalls))
	}
	items := b.chooseBufferCalls[0].items
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if !strings.Contains(items[0].Display, "5") {
		t.Errorf("display should contain size '5', got %q", items[0].Display)
	}
}
