package builtin_test

import (
	"strings"
	"testing"
)

func TestSetBuffer_HappyPath(t *testing.T) {
	b := newBackend()
	res := dispatch("set-buffer", []string{"-b", "foo", "hello"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if b.buffers["foo"] != "hello" {
		t.Errorf("SetBuffer not recorded; buffers=%v", b.buffers)
	}
}

func TestSetBuffer_AutoName(t *testing.T) {
	b := newBackend()
	res := dispatch("set-buffer", []string{"hello"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.buffers) != 1 {
		t.Fatalf("expected 1 buffer, got %d: %v", len(b.buffers), b.buffers)
	}
	// auto-name should be "buffer0"
	if _, ok := b.buffers["buffer0"]; !ok {
		t.Errorf("expected auto-named buffer 'buffer0'; buffers=%v", b.buffers)
	}
}

func TestDeleteBuffer_HappyPath(t *testing.T) {
	b := newBackend()
	_ = dispatch("set-buffer", []string{"-b", "foo", "hello"}, b)
	res := dispatch("delete-buffer", []string{"-b", "foo"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if _, ok := b.buffers["foo"]; ok {
		t.Error("buffer 'foo' should have been deleted")
	}
}

func TestDeleteBuffer_NotFound(t *testing.T) {
	b := newBackend()
	res := dispatch("delete-buffer", []string{"-b", "noexist"}, b)
	if res.Err == nil {
		t.Fatal("expected error when deleting non-existent buffer")
	}
}

func TestListBuffers_Empty(t *testing.T) {
	b := newBackend()
	res := dispatch("list-buffers", nil, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.Output != "" {
		t.Errorf("expected empty output for empty buffer list, got %q", res.Output)
	}
}

func TestListBuffers_WithBuffers(t *testing.T) {
	b := newBackend()
	_ = dispatch("set-buffer", []string{"-b", "foo", "hello"}, b)
	res := dispatch("list-buffers", nil, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !strings.Contains(res.Output, "foo") {
		t.Errorf("output missing buffer name 'foo': %q", res.Output)
	}
	if !strings.Contains(res.Output, "5 bytes") {
		t.Errorf("output missing '5 bytes': %q", res.Output)
	}
}

func TestLoadBuffer_HappyPath(t *testing.T) {
	b := newBackend()
	res := dispatch("load-buffer", []string{"-b", "foo", "/tmp/test.txt"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.loadedBuffers) != 1 {
		t.Fatalf("expected 1 loaded buffer, got %d", len(b.loadedBuffers))
	}
	got := b.loadedBuffers[0]
	if got.name != "foo" || got.path != "/tmp/test.txt" {
		t.Errorf("LoadBuffer called with name=%q path=%q", got.name, got.path)
	}
}

func TestSaveBuffer_HappyPath(t *testing.T) {
	b := newBackend()
	_ = dispatch("set-buffer", []string{"-b", "foo", "hello"}, b)
	res := dispatch("save-buffer", []string{"-b", "foo", "/tmp/out.txt"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.savedBuffers) != 1 {
		t.Fatalf("expected 1 saved buffer, got %d", len(b.savedBuffers))
	}
	got := b.savedBuffers[0]
	if got.name != "foo" || got.path != "/tmp/out.txt" {
		t.Errorf("SaveBuffer called with name=%q path=%q", got.name, got.path)
	}
}

func TestSaveBuffer_NotFound(t *testing.T) {
	b := newBackend()
	res := dispatch("save-buffer", []string{"-b", "noexist", "/tmp/out.txt"}, b)
	if res.Err == nil {
		t.Fatal("expected error when saving non-existent buffer")
	}
}

func TestPasteBuffer_HappyPath(t *testing.T) {
	b := newBackend()
	_ = dispatch("set-buffer", []string{"-b", "foo", "hello"}, b)
	res := dispatch("paste-buffer", []string{"-b", "foo"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(b.pastedBuffers) != 1 {
		t.Fatalf("expected 1 pasted buffer, got %d", len(b.pastedBuffers))
	}
	if b.pastedBuffers[0].name != "foo" {
		t.Errorf("PasteBuffer called with name=%q", b.pastedBuffers[0].name)
	}
}

func TestPasteBuffer_NotFound(t *testing.T) {
	b := newBackend()
	res := dispatch("paste-buffer", []string{"-b", "noexist"}, b)
	if res.Err == nil {
		t.Fatal("expected error when pasting non-existent buffer")
	}
}

func TestPasteBuffer_DeleteAfterPaste(t *testing.T) {
	b := newBackend()
	_ = dispatch("set-buffer", []string{"-b", "foo", "hello"}, b)
	res := dispatch("paste-buffer", []string{"-b", "foo", "-d"}, b)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if _, ok := b.buffers["foo"]; ok {
		t.Error("buffer 'foo' should have been deleted after paste with -d")
	}
}
