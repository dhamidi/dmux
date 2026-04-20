package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/options"
	"github.com/dhamidi/dmux/internal/session"
)

// TestBindKey verifies that BindKey stores a binding retrievable via Lookup.
func TestBindKey(t *testing.T) {
	m, _ := newTestMutator()

	if err := m.BindKey("root", "C-b", "prefix"); err != nil {
		t.Fatalf("BindKey: %v", err)
	}

	tbl, ok := m.state.KeyTables.Get("root")
	if !ok {
		t.Fatal("table 'root' not registered after BindKey")
	}
	k, err := keys.Parse("C-b")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cmd, ok := tbl.Lookup(k)
	if !ok {
		t.Error("Lookup returned false; want true")
	}
	if cmd != "prefix" {
		t.Errorf("Lookup = %v, want %q", cmd, "prefix")
	}
}

// TestUnbindKey verifies that UnbindKey removes an existing binding.
func TestUnbindKey(t *testing.T) {
	m, _ := newTestMutator()

	if err := m.BindKey("root", "C-b", "prefix"); err != nil {
		t.Fatalf("BindKey: %v", err)
	}
	if err := m.UnbindKey("root", "C-b"); err != nil {
		t.Fatalf("UnbindKey: %v", err)
	}

	tbl, _ := m.state.KeyTables.Get("root")
	k, _ := keys.Parse("C-b")
	if _, ok := tbl.Lookup(k); ok {
		t.Error("Lookup returned true after UnbindKey; want false")
	}
}

// TestUnbindKeyMissingTable checks that UnbindKey returns an error for an
// unknown table.
func TestUnbindKeyMissingTable(t *testing.T) {
	m, _ := newTestMutator()

	if err := m.UnbindKey("no-such-table", "C-b"); err == nil {
		t.Error("expected error for unknown table, got nil")
	}
}

// TestListKeyBindings verifies that ListKeyBindings("") returns bindings from
// all registered tables.
func TestListKeyBindings(t *testing.T) {
	m, _ := newTestMutator()

	if err := m.BindKey("root", "C-b", "prefix"); err != nil {
		t.Fatalf("BindKey root: %v", err)
	}
	if err := m.BindKey("prefix", "c", "new-window"); err != nil {
		t.Fatalf("BindKey prefix: %v", err)
	}

	bindings := m.ListKeyBindings("")
	if len(bindings) != 2 {
		t.Fatalf("ListKeyBindings(\"\") = %d bindings, want 2", len(bindings))
	}

	// Build a lookup map for easy assertion.
	byKey := make(map[string]string, len(bindings))
	for _, b := range bindings {
		byKey[b.Table+":"+b.Key] = b.Command
	}

	if cmd, ok := byKey["root:C-b"]; !ok || cmd != "prefix" {
		t.Errorf("root:C-b = %q (ok=%v), want %q", cmd, ok, "prefix")
	}
	if cmd, ok := byKey["prefix:c"]; !ok || cmd != "new-window" {
		t.Errorf("prefix:c = %q (ok=%v), want %q", cmd, ok, "new-window")
	}
}

// TestListKeyBindingsSingleTable verifies filtering by table name.
func TestListKeyBindingsSingleTable(t *testing.T) {
	m, _ := newTestMutator()

	_ = m.BindKey("root", "C-b", "prefix")
	_ = m.BindKey("prefix", "c", "new-window")

	bindings := m.ListKeyBindings("root")
	if len(bindings) != 1 {
		t.Fatalf("ListKeyBindings(\"root\") = %d, want 1", len(bindings))
	}
	if bindings[0].Table != "root" {
		t.Errorf("Table = %q, want %q", bindings[0].Table, "root")
	}
}

// TestSetOption_Server verifies setting an option on the server scope.
func TestSetOption_Server(t *testing.T) {
	m, _ := newTestMutator()

	if err := m.SetOption("server", "status", "on"); err != nil {
		t.Fatalf("SetOption: %v", err)
	}
	v, ok := m.state.Options.Get("status")
	if !ok {
		t.Fatal("Get returned false; want true")
	}
	if v.String() != "on" {
		t.Errorf("value = %q, want %q", v.String(), "on")
	}
}

// TestSetOption_Session verifies setting an option on a session scope.
func TestSetOption_Session(t *testing.T) {
	m, _ := newTestMutator()

	sv, err := m.NewSession("alpha")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	scope := "session:" + sv.ID
	if err := m.SetOption(scope, "remain-on-exit", "off"); err != nil {
		t.Fatalf("SetOption: %v", err)
	}

	sess := m.state.Sessions[session.SessionID(sv.ID)]
	v, ok := sess.Options.Get("remain-on-exit")
	if !ok {
		t.Fatal("Get returned false; want true")
	}
	if v.String() != "off" {
		t.Errorf("value = %q, want %q", v.String(), "off")
	}
}

// TestUnsetOption verifies that UnsetOption removes a locally-set value.
func TestUnsetOption(t *testing.T) {
	m, _ := newTestMutator()

	if err := m.SetOption("server", "mouse", "on"); err != nil {
		t.Fatalf("SetOption: %v", err)
	}
	if err := m.UnsetOption("server", "mouse"); err != nil {
		t.Fatalf("UnsetOption: %v", err)
	}

	// After unset the value should fall through to the registered default ("").
	v, ok := m.state.Options.Get("mouse")
	if !ok {
		t.Fatal("Get returned false after Unset; option should still be registered")
	}
	if v.Kind != options.String || v.Str != "" {
		t.Errorf("value after Unset = %q, want empty string default", v.String())
	}
}

// TestListOptions verifies that ListOptions returns all locally-set options.
func TestListOptions(t *testing.T) {
	m, _ := newTestMutator()

	_ = m.SetOption("server", "opt-a", "val-a")
	_ = m.SetOption("server", "opt-b", "val-b")

	entries := m.ListOptions("server")
	if len(entries) != 2 {
		t.Fatalf("ListOptions = %d entries, want 2", len(entries))
	}

	byName := make(map[string]string, len(entries))
	for _, e := range entries {
		byName[e.Name] = e.Value
	}
	if byName["opt-a"] != "val-a" {
		t.Errorf("opt-a = %q, want %q", byName["opt-a"], "val-a")
	}
	if byName["opt-b"] != "val-b" {
		t.Errorf("opt-b = %q, want %q", byName["opt-b"], "val-b")
	}
}
