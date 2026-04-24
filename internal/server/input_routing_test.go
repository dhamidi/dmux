package server

import (
	"bytes"
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/termcaps"
	"github.com/dhamidi/dmux/internal/termin"
)

// fakePane records every byte written to it. Used as the
// passthrough target routeInput forwards unbound keys to.
type fakePane struct {
	buf bytes.Buffer
}

func (p *fakePane) Write(b []byte) (int, error) {
	return p.buf.Write(b)
}

// fakeDispatcher records every argv it was asked to run.
type fakeDispatcher struct {
	calls [][]string
}

func (d *fakeDispatcher) dispatch(argv []string) {
	// Copy the slice to avoid aliasing the caller's buffer.
	cp := make([]string, len(argv))
	copy(cp, argv)
	d.calls = append(d.calls, cp)
}

// buildTestTables returns a (root, prefix, named-map) triple with
// "C-b" → switch to "prefix" in root, and "n"/"p"/"c" → window
// commands in prefix.
func buildTestTables() (*keys.Table, *keys.Table, map[string]*keys.Table) {
	root := keys.NewTable("root")
	prefix := keys.NewTable("prefix")

	root.Bind(&keys.Binding{
		Key:         keys.KeyCode{Key: keys.KeyB, Mods: keys.ModCtrl},
		SwitchTable: "prefix",
	})
	prefix.Bind(&keys.Binding{
		Key:  keys.KeyCode{Key: keys.KeyN},
		Argv: []string{"next-window"},
	})
	prefix.Bind(&keys.Binding{
		Key:  keys.KeyCode{Key: keys.KeyP},
		Argv: []string{"previous-window"},
	})
	prefix.Bind(&keys.Binding{
		Key:  keys.KeyCode{Key: keys.KeyC},
		Argv: []string{"new-window"},
	})

	return root, prefix, map[string]*keys.Table{
		"root":   root,
		"prefix": prefix,
	}
}

// An unbound printable key is forwarded to the pane verbatim and
// leaves the current table alone.
func TestRouteInputForwardsUnboundKey(t *testing.T) {
	root, _, tables := buildTestTables()
	parser := termin.NewParser(termcaps.Unknown)
	pane := &fakePane{}
	disp := &fakeDispatcher{}

	em := parser.Feed([]byte{'a'})
	out := routeInput(em, root, root, tables, disp, pane)

	if out != root {
		t.Fatalf("routeInput: table changed unexpectedly to %q", out.Name)
	}
	if got := pane.buf.Bytes(); !bytes.Equal(got, []byte{'a'}) {
		t.Fatalf("pane received %q, want %q", got, "a")
	}
	if len(disp.calls) != 0 {
		t.Fatalf("dispatcher fired for unbound key: %v", disp.calls)
	}
}

// C-b in the root table switches to the prefix table without
// forwarding any bytes or firing a command.
func TestRouteInputSwitchTableSwallowsKey(t *testing.T) {
	root, prefix, tables := buildTestTables()
	parser := termin.NewParser(termcaps.Unknown)
	pane := &fakePane{}
	disp := &fakeDispatcher{}

	em := parser.Feed([]byte{0x02}) // Ctrl-B
	out := routeInput(em, root, root, tables, disp, pane)

	if out != prefix {
		t.Fatalf("routeInput: current table = %q, want prefix", out.Name)
	}
	if pane.buf.Len() != 0 {
		t.Fatalf("pane received bytes for bound C-b: %q", pane.buf.Bytes())
	}
	if len(disp.calls) != 0 {
		t.Fatalf("dispatcher fired for a SwitchTable binding: %v", disp.calls)
	}
}

// In the prefix table, 'n' fires next-window and the pump returns
// to the root table. One-shot prefix semantics (tmux default).
func TestRouteInputCommandBindingReturnsToRoot(t *testing.T) {
	root, prefix, tables := buildTestTables()
	parser := termin.NewParser(termcaps.Unknown)
	pane := &fakePane{}
	disp := &fakeDispatcher{}

	em := parser.Feed([]byte{'n'})
	out := routeInput(em, prefix, root, tables, disp, pane)

	if out != root {
		t.Fatalf("routeInput: current table = %q, want root (one-shot prefix)", out.Name)
	}
	if pane.buf.Len() != 0 {
		t.Fatalf("pane received bytes for bound n: %q", pane.buf.Bytes())
	}
	if len(disp.calls) != 1 {
		t.Fatalf("dispatcher fired %d times, want 1: %v", len(disp.calls), disp.calls)
	}
	if got := disp.calls[0]; len(got) != 1 || got[0] != "next-window" {
		t.Fatalf("dispatcher argv = %v, want [next-window]", got)
	}
}

// A bound key in the prefix table that isn't bound here must still
// forward to the pane rather than silently swallowing.
func TestRouteInputPrefixMissForwardsBytes(t *testing.T) {
	root, prefix, tables := buildTestTables()
	parser := termin.NewParser(termcaps.Unknown)
	pane := &fakePane{}
	disp := &fakeDispatcher{}

	// 'z' isn't bound in prefix.
	em := parser.Feed([]byte{'z'})
	out := routeInput(em, prefix, root, tables, disp, pane)

	if out != prefix {
		t.Fatalf("routeInput: unbound key in prefix moved table to %q", out.Name)
	}
	if got := pane.buf.Bytes(); !bytes.Equal(got, []byte{'z'}) {
		t.Fatalf("pane received %q, want %q", got, "z")
	}
	if len(disp.calls) != 0 {
		t.Fatalf("dispatcher fired for unbound key in prefix: %v", disp.calls)
	}
}

// The end-to-end "C-b n" sequence: after two Feed calls the
// dispatcher fires next-window exactly once and the pump returns to
// the root table.
func TestRouteInputChordCtrlBThenN(t *testing.T) {
	root, _, tables := buildTestTables()
	parser := termin.NewParser(termcaps.Unknown)
	pane := &fakePane{}
	disp := &fakeDispatcher{}

	cur := root
	cur = routeInput(parser.Feed([]byte{0x02}), cur, root, tables, disp, pane)
	cur = routeInput(parser.Feed([]byte{'n'}), cur, root, tables, disp, pane)

	if cur != root {
		t.Fatalf("final table = %q, want root", cur.Name)
	}
	if pane.buf.Len() != 0 {
		t.Fatalf("pane received bytes during prefix chord: %q", pane.buf.Bytes())
	}
	if len(disp.calls) != 1 {
		t.Fatalf("dispatcher fired %d times, want 1: %v", len(disp.calls), disp.calls)
	}
	if disp.calls[0][0] != "next-window" {
		t.Fatalf("fired %v, want next-window", disp.calls[0])
	}
}

// If a SwitchTable names a table that is not in the map, routeInput
// falls back to the root table rather than leaving the pump stuck
// on the invalid reference.
func TestRouteInputSwitchTableUnknownFallsBack(t *testing.T) {
	root := keys.NewTable("root")
	root.Bind(&keys.Binding{
		Key:         keys.KeyCode{Key: keys.KeyA},
		SwitchTable: "nowhere",
	})
	tables := map[string]*keys.Table{"root": root}

	parser := termin.NewParser(termcaps.Unknown)
	pane := &fakePane{}
	disp := &fakeDispatcher{}

	em := parser.Feed([]byte{'a'})
	out := routeInput(em, root, root, tables, disp, pane)

	if out != root {
		t.Fatalf("routeInput: fallback table = %q, want root", out.Name)
	}
}
