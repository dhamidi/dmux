// Package options is a hierarchical, typed key-value store with
// parent/child inheritance.
//
// # Boundary
//
// The package depends only on the Go standard library.  It has no imports
// from other internal/* packages and can be used in isolation.
//
// # Store and Value
//
// A Store holds name → Value mappings.  Value is a sum type over four kinds:
//
//   - options.String  – a Go string
//   - options.Int     – a Go int
//   - options.Bool    – a Go bool
//   - options.Strings – a Go []string
//
// # Registration
//
// Option names and their types are not hardcoded.  Consumers register them:
//
//	s.Register("status-bg", options.String, "green")
//	s.Register("history-limit", options.Int, 2000)
//	s.Register("mouse", options.Bool, false)
//	s.Register("env", options.Strings, []string{})
//
// Registering the same name with the same Kind is idempotent (the first
// default wins).  Registering the same name with a different Kind panics.
//
// # Get and Set
//
// Values are read with Get (returns a Value and an ok bool) or the typed
// helpers GetString, GetInt, GetBool, GetStrings.  Values are written with
// Set(name, goValue) which returns an error if the name is unregistered.
// Unset(name) removes a local override so lookups fall through to the parent.
//
// # Inheritance
//
// New() creates a root Store.  NewChild(parent) creates a child whose lookups
// fall through to the parent chain when a key is not set locally.  The parent
// reference is a *Store; no other types are involved.
//
// The intended hierarchy for dmux:
//
//	server  (root Store)
//	  └── session  (NewChild(server))
//	        └── window  (NewChild(session))
//	              └── pane  (NewChild(window))
//
// A pane's effective value for any option is the first ancestor that has it
// set locally; if no ancestor has it set the registered default is returned.
//
// # Iteration
//
// Each(fn) calls fn for every locally-set key in the Store (parent values are
// not included).  Iteration order is unspecified.
package options
