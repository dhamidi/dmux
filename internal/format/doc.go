// Package format expands #{...} format strings.
//
// # Boundary
//
// Expand(template string, ctx Context) (string, error) is the entire
// public surface. Context is an interface:
//
//	type Context interface {
//	    Lookup(key string) (string, bool)
//	    Children(listKey string) []Context
//	}
//
// The template syntax mirrors tmux:
//   - #{var}                  simple substitution
//   - #{?cond,yes,no}         conditional
//   - #{=20:var}              truncate
//   - #{s/from/to/:var}       regex substitute
//   - #{||:a,b}  #{&&:a,b}    boolean combinators
//   - #{T:var}  #{t:var}      time formatting
//   - #(shell-cmd)            external command (via package job)
//
// Because Context is an interface, package session provides the bindings
// when expanding for a status line or display-message, but format itself
// has no dependency on session and can be used with a plain
// map[string]string for testing or for any string-templating task.
//
// # In isolation
//
// Reusable as a general templating library. Shipped with a command-line
// example that takes a template and a set of KEY=VALUE pairs and prints
// the expansion.
//
// # Non-goals
//
// No rendering (produces plain strings; package status turns them into
// cells). No caching of expensive expansions — callers that care cache
// their own results.
package format
