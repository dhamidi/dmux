package keys

// BoundCommand is the opaque command stored in a key binding. The
// keys package does not interpret this value; it stores and returns
// it exactly as provided by the caller.
//
// In practice callers store a raw command string (the text to be
// parsed and dispatched by the server loop), keeping this package
// free of any dependency on internal/command.
type BoundCommand = any

// Table maps [Key] values to [BoundCommand] values. It represents a
// single named key table such as "root", "prefix", or "copy-mode-vi".
//
// Table is not safe for concurrent use.
type Table struct {
	bindings map[Key]BoundCommand
}

// NewTable returns an empty Table.
func NewTable() *Table {
	return &Table{bindings: make(map[Key]BoundCommand)}
}

// Bind adds or replaces the binding for key with cmd.
func (t *Table) Bind(key Key, cmd BoundCommand) {
	t.bindings[key] = cmd
}

// Unbind removes the binding for key. It is a no-op if no binding exists.
func (t *Table) Unbind(key Key) {
	delete(t.bindings, key)
}

// Lookup returns the command bound to key and true, or nil and false
// if no binding exists.
func (t *Table) Lookup(key Key) (BoundCommand, bool) {
	cmd, ok := t.bindings[key]
	return cmd, ok
}

// Len returns the number of bindings in the table.
func (t *Table) Len() int {
	return len(t.bindings)
}

// Each calls fn for every binding in the table. Order is undefined.
// Modifying the table from within fn is not supported.
func (t *Table) Each(fn func(key Key, cmd BoundCommand)) {
	for k, v := range t.bindings {
		fn(k, v)
	}
}

// Registry maps table names to [Table] values. A [Registry] is the
// canonical store for all key tables in a dmux server.
//
// Registry is not safe for concurrent use.
type Registry struct {
	tables map[string]*Table
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tables: make(map[string]*Table)}
}

// Register adds or replaces the table for name.
func (r *Registry) Register(name string, t *Table) {
	r.tables[name] = t
}

// Get returns the table for name and true, or nil and false if no
// table with that name has been registered.
func (r *Registry) Get(name string) (*Table, bool) {
	t, ok := r.tables[name]
	return t, ok
}

// Remove deletes the table registered under name. It is a no-op if
// no table exists for that name.
func (r *Registry) Remove(name string) {
	delete(r.tables, name)
}

// Names returns all registered table names in undefined order.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.tables))
	for n := range r.tables {
		names = append(names, n)
	}
	return names
}
