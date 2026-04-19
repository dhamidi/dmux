// Package options is a hierarchical, typed key-value store with
// parent/child inheritance.
//
// # Boundary
//
// A Store holds name → Value. Value is a sum type over string, int,
// bool, and []string. New(parent *Store) returns a child Store whose
// lookups fall through to the parent if a key is unset locally.
//
// The intended hierarchy for dmux:
//
//	server  (global)
//	  └── session
//	        └── window
//	              └── pane
//
// Each session is constructed with the server Store as parent; each
// window with its session as parent; and so on. A pane's effective
// value for any option is its first ancestor that sets it.
//
// # Registration
//
// Option names and their types aren't hardcoded in this package. Consumers
// register them: options.Register("status-bg", options.String, "green").
// This keeps this package agnostic to the actual option set and makes it
// reusable as a generic config store.
//
// # In isolation
//
// Can be dropped into any Go program that needs a scoped config tree.
// No dependency on the rest of dmux.
//
// # Non-goals
//
// No file I/O. No parsing of user config. That's cfg.c in tmux; in
// dmux, the config file is parsed by package parse into a CommandList
// of `set-option` commands, which mutate the Store.
package options
