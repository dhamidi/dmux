package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/session"
)

func TestLoadDefaultBindings(t *testing.T) {
	state := session.NewServer()
	state.KeyTables = keys.NewRegistry()
	mut := newServerMutator(state, func() {},
		func(session.ClientID) (*clientConn, bool) { return nil, false },
		func(*clientConn) {},
	)

	if err := loadDefaultBindings(mut); err != nil {
		t.Fatalf("loadDefaultBindings: %v", err)
	}

	prefixTable, ok := state.KeyTables.Get("prefix")
	if !ok {
		t.Fatal("prefix table not registered")
	}
	for _, keyStr := range []string{"c", "d", "n", "%"} {
		k, err := keys.Parse(keyStr)
		if err != nil {
			t.Fatalf("keys.Parse(%q): %v", keyStr, err)
		}
		if _, bound := prefixTable.Lookup(k); !bound {
			t.Errorf("expected prefix+%q to be bound", keyStr)
		}
	}

	// Also verify the root table has C-b bound.
	rootTable, ok := state.KeyTables.Get("root")
	if !ok {
		t.Fatal("root table not registered")
	}
	cbKey, err := keys.Parse("C-b")
	if err != nil {
		t.Fatalf("keys.Parse(%q): %v", "C-b", err)
	}
	if _, bound := rootTable.Lookup(cbKey); !bound {
		t.Error("expected root+C-b to be bound")
	}
}
