package options_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/options"
)

// helpers

func mustSet(t *testing.T, s *options.Store, name string, value interface{}) {
	t.Helper()
	if err := s.Set(name, value); err != nil {
		t.Fatalf("Set(%q, %v): %v", name, value, err)
	}
}

// ---------------------------------------------------------------------------
// Basic get/set on a root store
// ---------------------------------------------------------------------------

func TestRootStore_String(t *testing.T) {
	s := options.New()
	s.Register("color", options.String, "blue")

	// default
	v, ok := s.GetString("color")
	if !ok || v != "blue" {
		t.Fatalf("want (blue, true), got (%q, %v)", v, ok)
	}

	// set
	mustSet(t, s, "color", "red")
	v, ok = s.GetString("color")
	if !ok || v != "red" {
		t.Fatalf("after Set: want (red, true), got (%q, %v)", v, ok)
	}
}

func TestRootStore_Int(t *testing.T) {
	s := options.New()
	s.Register("width", options.Int, 80)

	n, ok := s.GetInt("width")
	if !ok || n != 80 {
		t.Fatalf("want (80, true), got (%d, %v)", n, ok)
	}

	mustSet(t, s, "width", 120)
	n, ok = s.GetInt("width")
	if !ok || n != 120 {
		t.Fatalf("after Set: want (120, true), got (%d, %v)", n, ok)
	}
}

func TestRootStore_Bool(t *testing.T) {
	s := options.New()
	s.Register("verbose", options.Bool, false)

	b, ok := s.GetBool("verbose")
	if !ok || b != false {
		t.Fatalf("want (false, true), got (%v, %v)", b, ok)
	}

	mustSet(t, s, "verbose", true)
	b, ok = s.GetBool("verbose")
	if !ok || b != true {
		t.Fatalf("after Set: want (true, true), got (%v, %v)", b, ok)
	}
}

func TestRootStore_Strings(t *testing.T) {
	s := options.New()
	s.Register("hooks", options.Strings, []string{"a", "b"})

	list, ok := s.GetStrings("hooks")
	if !ok || len(list) != 2 || list[0] != "a" || list[1] != "b" {
		t.Fatalf("want ([a b], true), got (%v, %v)", list, ok)
	}

	mustSet(t, s, "hooks", []string{"x"})
	list, ok = s.GetStrings("hooks")
	if !ok || len(list) != 1 || list[0] != "x" {
		t.Fatalf("after Set: want ([x], true), got (%v, %v)", list, ok)
	}
}

// ---------------------------------------------------------------------------
// Unregistered key
// ---------------------------------------------------------------------------

func TestGet_UnregisteredKey(t *testing.T) {
	s := options.New()
	_, ok := s.Get("nonexistent")
	if ok {
		t.Fatal("Get on unregistered key should return ok=false")
	}
}

func TestSet_UnregisteredKey(t *testing.T) {
	s := options.New()
	err := s.Set("nonexistent", "value")
	if err == nil {
		t.Fatal("Set on unregistered key should return an error")
	}
}

// ---------------------------------------------------------------------------
// Parent/child inheritance
// ---------------------------------------------------------------------------

func TestChild_FallsBackToParent(t *testing.T) {
	parent := options.New()
	parent.Register("color", options.String, "blue")
	mustSet(t, parent, "color", "green")

	child := options.NewChild(parent)

	v, ok := child.GetString("color")
	if !ok || v != "green" {
		t.Fatalf("child should inherit parent value; got (%q, %v)", v, ok)
	}
}

func TestChild_OverridesParent(t *testing.T) {
	parent := options.New()
	parent.Register("color", options.String, "blue")
	mustSet(t, parent, "color", "green")

	child := options.NewChild(parent)
	mustSet(t, child, "color", "red")

	// child has override
	v, ok := child.GetString("color")
	if !ok || v != "red" {
		t.Fatalf("child override: want (red, true), got (%q, %v)", v, ok)
	}

	// parent is unchanged
	pv, _ := parent.GetString("color")
	if pv != "green" {
		t.Fatalf("parent should be unchanged; got %q", pv)
	}
}

func TestChild_UsesDefault_WhenParentUnset(t *testing.T) {
	parent := options.New()
	parent.Register("timeout", options.Int, 30)

	child := options.NewChild(parent)

	n, ok := child.GetInt("timeout")
	if !ok || n != 30 {
		t.Fatalf("want (30, true), got (%d, %v)", n, ok)
	}
}

func TestChild_Unset_FallsBackToParent(t *testing.T) {
	parent := options.New()
	parent.Register("color", options.String, "blue")
	mustSet(t, parent, "color", "green")

	child := options.NewChild(parent)
	mustSet(t, child, "color", "red")
	child.Unset("color")

	v, ok := child.GetString("color")
	if !ok || v != "green" {
		t.Fatalf("after Unset: want parent value (green, true), got (%q, %v)", v, ok)
	}
}

// ---------------------------------------------------------------------------
// Multi-level inheritance
// ---------------------------------------------------------------------------

func TestMultiLevel_Inheritance(t *testing.T) {
	// server → session → window → pane
	server := options.New()
	server.Register("status-bg", options.String, "default")
	mustSet(t, server, "status-bg", "black")

	session := options.NewChild(server)
	// session does not override

	window := options.NewChild(session)
	// window does not override

	pane := options.NewChild(window)
	// pane does not override

	v, ok := pane.GetString("status-bg")
	if !ok || v != "black" {
		t.Fatalf("pane should inherit server value; got (%q, %v)", v, ok)
	}

	// override at window level
	mustSet(t, window, "status-bg", "red")
	v, ok = pane.GetString("status-bg")
	if !ok || v != "red" {
		t.Fatalf("pane should see window override; got (%q, %v)", v, ok)
	}

	// override at pane level wins
	mustSet(t, pane, "status-bg", "blue")
	v, ok = pane.GetString("status-bg")
	if !ok || v != "blue" {
		t.Fatalf("pane override should win; got (%q, %v)", v, ok)
	}

	// intermediate levels unchanged
	sv, _ := server.GetString("status-bg")
	if sv != "black" {
		t.Fatalf("server should still be black; got %q", sv)
	}
	wv, _ := window.GetString("status-bg")
	if wv != "red" {
		t.Fatalf("window should still be red; got %q", wv)
	}
}

// ---------------------------------------------------------------------------
// Each iterates local values only
// ---------------------------------------------------------------------------

func TestEach_LocalOnly(t *testing.T) {
	parent := options.New()
	parent.Register("a", options.String, "")
	parent.Register("b", options.Int, 0)
	mustSet(t, parent, "a", "parent-a")
	mustSet(t, parent, "b", 42)

	child := options.NewChild(parent)
	mustSet(t, child, "a", "child-a")

	seen := map[string]options.Value{}
	child.Each(func(name string, v options.Value) {
		seen[name] = v
	})

	if len(seen) != 1 {
		t.Fatalf("Each should yield only local keys; got %d keys: %v", len(seen), seen)
	}
	if seen["a"].Str != "child-a" {
		t.Fatalf("Each: want child-a, got %q", seen["a"].Str)
	}
}

// ---------------------------------------------------------------------------
// Value.String()
// ---------------------------------------------------------------------------

func TestValue_String(t *testing.T) {
	cases := []struct {
		v    options.Value
		want string
	}{
		{options.Value{Kind: options.String, Str: "hello"}, "hello"},
		{options.Value{Kind: options.Int, Integer: 42}, "42"},
		{options.Value{Kind: options.Bool, Flag: true}, "on"},
		{options.Value{Kind: options.Bool, Flag: false}, "off"},
		{options.Value{Kind: options.Strings, List: []string{"a", "b"}}, "[a b]"},
	}
	for _, c := range cases {
		if got := c.v.String(); got != c.want {
			t.Errorf("Value.String() = %q, want %q", got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Type mismatch helpers return ok=false
// ---------------------------------------------------------------------------

func TestGetWrongKind(t *testing.T) {
	s := options.New()
	s.Register("flag", options.Bool, true)

	if _, ok := s.GetString("flag"); ok {
		t.Fatal("GetString on Bool key should return ok=false")
	}
	if _, ok := s.GetInt("flag"); ok {
		t.Fatal("GetInt on Bool key should return ok=false")
	}
	if _, ok := s.GetStrings("flag"); ok {
		t.Fatal("GetStrings on Bool key should return ok=false")
	}
}

// ---------------------------------------------------------------------------
// Register is idempotent for the same kind
// ---------------------------------------------------------------------------

func TestRegister_Idempotent(t *testing.T) {
	s := options.New()
	s.Register("x", options.String, "a")
	s.Register("x", options.String, "b") // same kind — no panic, default keeps first

	v, _ := s.GetString("x")
	if v != "a" {
		t.Fatalf("idempotent Register should keep first default; got %q", v)
	}
}

func TestRegister_PanicsOnKindMismatch(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on kind mismatch")
		}
	}()
	s := options.New()
	s.Register("x", options.String, "a")
	s.Register("x", options.Int, 1) // different kind — must panic
}

// ---------------------------------------------------------------------------
// Child registers its own definitions (not leaked to parent)
// ---------------------------------------------------------------------------

func TestChild_RegisterOwnDefs(t *testing.T) {
	parent := options.New()
	parent.Register("shared", options.String, "default")

	child := options.NewChild(parent)
	child.Register("child-only", options.Int, 99)

	// child can use child-only
	n, ok := child.GetInt("child-only")
	if !ok || n != 99 {
		t.Fatalf("child-only: want (99, true), got (%d, %v)", n, ok)
	}

	// parent cannot see child-only
	_, ok = parent.Get("child-only")
	if ok {
		t.Fatal("parent should not see child-only key")
	}
}
