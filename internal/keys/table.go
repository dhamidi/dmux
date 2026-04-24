package keys

// Binding associates a KeyCode with a command invocation to run when
// the key fires. Argv is stored pre-parsed: the server looks argv[0]
// up in the cmd registry and hands the whole slice to Exec. Multi-
// command bindings (tmux's "a ; b") are deferred; one argv per
// binding today.
//
// Note is the free-form description shown by list-keys; it carries
// no semantics. Repeat controls whether the binding keeps firing
// while the user holds the key inside the "repeat" window after
// entering it — the server implements that policy; this package
// only records the bit.
type Binding struct {
	// Key is the normalized trigger for this binding.
	Key KeyCode
	// Argv is the command invocation, including argv[0] (the command
	// name). It is never nil for an installed binding; empty argvs
	// are a caller bug.
	Argv []string
	// Note is a human-readable description surfaced by list-keys.
	Note string
	// Repeat marks the binding as eligible for key-repeat within the
	// server's repeat window.
	Repeat bool
}

// Table is a named set of KeyCode->Binding mappings. A server holds
// several at once (root, prefix, copy-mode, ...) and consults the
// current table based on per-client state. Lookups are a single map
// access — any byte-sequence complexity lives upstream in termin.
//
// Table is not safe for concurrent use; callers coordinate access
// through the server's main goroutine, same as every other piece of
// server state.
type Table struct {
	// Name identifies the table in list-keys output and in commands
	// that target a table by name ("bind-key -T <name>"). It is not
	// used for lookup.
	Name string
	// Bindings is the KeyCode index into installed bindings. A nil
	// map is equivalent to an empty one for Lookup, but Bind lazily
	// allocates before inserting.
	Bindings map[KeyCode]*Binding
}

// NewTable returns an empty table named name. The backing map is
// allocated up front so the zero-allocation path is Bind, not
// NewTable.
func NewTable(name string) *Table {
	return &Table{
		Name:     name,
		Bindings: make(map[KeyCode]*Binding),
	}
}

// Bind adds b to the table, replacing any existing binding on the
// same KeyCode. Returns the replaced binding, or nil if none. Bind
// allocates the backing map on first use so zero-value Tables work
// even without NewTable (useful in tests).
func (t *Table) Bind(b *Binding) *Binding {
	if t.Bindings == nil {
		t.Bindings = make(map[KeyCode]*Binding)
	}
	prev := t.Bindings[b.Key]
	t.Bindings[b.Key] = b
	return prev
}

// Unbind removes the binding for code and returns it, or nil if no
// binding existed.
func (t *Table) Unbind(code KeyCode) *Binding {
	if t.Bindings == nil {
		return nil
	}
	prev, ok := t.Bindings[code]
	if !ok {
		return nil
	}
	delete(t.Bindings, code)
	return prev
}

// Lookup returns the binding for code, or nil if unbound. Lookup on
// a nil Bindings map returns nil rather than panicking so a
// zero-value Table behaves like an empty one.
func (t *Table) Lookup(code KeyCode) *Binding {
	if t.Bindings == nil {
		return nil
	}
	return t.Bindings[code]
}
