package options

import "fmt"

// Kind identifies the type of an option's value.
type Kind int

const (
	String  Kind = iota // string value
	Int                 // int value
	Bool                // bool value
	Strings             // []string value
	Style               // style string, e.g. "bg=green,fg=black"
)

// Value holds one option value. Only the field matching Kind is valid.
type Value struct {
	Kind    Kind
	Str     string
	Integer int
	Flag    bool
	List    []string
}

func (v Value) String() string {
	switch v.Kind {
	case String:
		return v.Str
	case Style:
		return v.Str
	case Int:
		return fmt.Sprintf("%d", v.Integer)
	case Bool:
		if v.Flag {
			return "on"
		}
		return "off"
	case Strings:
		return fmt.Sprintf("%v", v.List)
	default:
		return ""
	}
}

// definition records the type and default value for a registered option.
type definition struct {
	kind     Kind
	defaults Value
}

// Store is a scoped key-value store. Each Store may have a parent; lookups
// that find no local value fall through to the parent chain.
type Store struct {
	parent  *Store
	defs    map[string]definition // registered options (shared up the chain)
	values  map[string]Value      // locally-set values
}

// New returns a new Store with no parent (a root store).
func New() *Store {
	return &Store{
		defs:   make(map[string]definition),
		values: make(map[string]Value),
	}
}

// NewChild returns a new Store whose parent is s. Lookups that find no local
// value fall through to the parent chain. Option registrations should be
// performed on the root store before creating children.
func NewChild(parent *Store) *Store {
	return &Store{
		parent: parent,
		defs:   make(map[string]definition),
		values: make(map[string]Value),
	}
}

// Register declares an option name, its kind, and its default value.
// It is idempotent: registering the same name twice with the same kind is a
// no-op; registering with a different kind panics.
func (s *Store) Register(name string, kind Kind, defaultValue interface{}) {
	def, exists := s.defs[name]
	if exists {
		if def.kind != kind {
			panic(fmt.Sprintf("options: Register %q: kind mismatch (was %v, got %v)", name, def.kind, kind))
		}
		return
	}
	s.defs[name] = definition{kind: kind, defaults: toValue(kind, defaultValue)}
}

// toValue converts a Go value to a Value with the given Kind.
func toValue(kind Kind, v interface{}) Value {
	switch kind {
	case String:
		switch x := v.(type) {
		case string:
			return Value{Kind: String, Str: x}
		case nil:
			return Value{Kind: String}
		}
	case Int:
		switch x := v.(type) {
		case int:
			return Value{Kind: Int, Integer: x}
		case int64:
			return Value{Kind: Int, Integer: int(x)}
		case nil:
			return Value{Kind: Int}
		}
	case Bool:
		switch x := v.(type) {
		case bool:
			return Value{Kind: Bool, Flag: x}
		case nil:
			return Value{Kind: Bool}
		}
	case Strings:
		switch x := v.(type) {
		case []string:
			cp := make([]string, len(x))
			copy(cp, x)
			return Value{Kind: Strings, List: cp}
		case nil:
			return Value{Kind: Strings}
		}
	case Style:
		switch x := v.(type) {
		case string:
			return Value{Kind: Style, Str: x}
		case nil:
			return Value{Kind: Style}
		}
	}
	panic(fmt.Sprintf("options: toValue: cannot convert %T to kind %v", v, kind))
}

// lookupDef searches this store and its ancestors for a definition.
func (s *Store) lookupDef(name string) (definition, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if def, ok := cur.defs[name]; ok {
			return def, true
		}
	}
	return definition{}, false
}

// Get returns the effective Value for name, searching from this store up
// through the parent chain. If no ancestor has the key set, the registered
// default is returned. If the name is not registered, the second return value
// is false.
func (s *Store) Get(name string) (Value, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if v, ok := cur.values[name]; ok {
			return v, true
		}
	}
	// fall back to default
	def, ok := s.lookupDef(name)
	if !ok {
		return Value{}, false
	}
	return def.defaults, true
}

// Set stores value for name in this Store (not the parent). The name must have
// been registered and the value must match the registered kind.
func (s *Store) Set(name string, value interface{}) error {
	def, ok := s.lookupDef(name)
	if !ok {
		return fmt.Errorf("options: Set %q: not registered", name)
	}
	v := toValue(def.kind, value)
	s.values[name] = v
	return nil
}

// Unset removes the local override for name in this Store, so lookups will
// fall through to the parent again.
func (s *Store) Unset(name string) {
	delete(s.values, name)
}

// GetString returns the string value of name, or ("", false) if not found or
// not a String kind.
func (s *Store) GetString(name string) (string, bool) {
	v, ok := s.Get(name)
	if !ok || v.Kind != String {
		return "", false
	}
	return v.Str, true
}

// GetInt returns the int value of name, or (0, false) if not found or not an
// Int kind.
func (s *Store) GetInt(name string) (int, bool) {
	v, ok := s.Get(name)
	if !ok || v.Kind != Int {
		return 0, false
	}
	return v.Integer, true
}

// GetBool returns the bool value of name, or (false, false) if not found or
// not a Bool kind.
func (s *Store) GetBool(name string) (bool, bool) {
	v, ok := s.Get(name)
	if !ok || v.Kind != Bool {
		return false, false
	}
	return v.Flag, true
}

// GetStyle returns the style string value of name, or ("", false) if not found
// or not a Style kind.
func (s *Store) GetStyle(name string) (string, bool) {
	v, ok := s.Get(name)
	if !ok || v.Kind != Style {
		return "", false
	}
	return v.Str, true
}

// GetStrings returns the []string value of name, or (nil, false) if not found
// or not a Strings kind.
func (s *Store) GetStrings(name string) ([]string, bool) {
	v, ok := s.Get(name)
	if !ok || v.Kind != Strings {
		return nil, false
	}
	return v.List, true
}

// Each calls fn for every locally-set key in s (does not include parent
// values). Iteration order is not defined.
func (s *Store) Each(fn func(name string, value Value)) {
	for k, v := range s.values {
		fn(k, v)
	}
}
