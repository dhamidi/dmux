// Package format expands #{...} format strings.
//
// # Boundary
//
// Expand(template string, env Env) (string, error). Env bundles the two
// interfaces this package needs:
//
//	type Env struct {
//	    Ctx   Context     // variable lookups
//	    Shell ShellRunner // for #(...); nil disables shell expansion
//	    Now   func() time.Time   // optional; defaults to time.Now
//	}
//
//	type Context interface {
//	    Lookup(key string) (string, bool)
//	    Children(listKey string) []Context
//	}
//
//	type ShellRunner interface {
//	    Run(cmdline string) (stdout string, err error)
//	}
//
// The template syntax mirrors tmux:
//   - #{var}                  simple substitution
//   - #{?cond,yes,no}         conditional
//   - #{=20:var}              truncate
//   - #{s/from/to/:var}       regex substitute
//   - #{||:a,b}  #{&&:a,b}    boolean combinators
//   - #{T:var}  #{t:var}      time formatting
//   - #(shell-cmd)            external command (delegated to ShellRunner)
//
// Because every collaborator is an interface, package session provides
// Context when expanding a status line, package job satisfies ShellRunner
// in production, and tests pass a plain map[string]string Context plus
// a fake ShellRunner. format itself imports neither session nor job.
//
// # I/O surfaces
//
// None of its own. Any I/O implied by #(...) happens in the caller's
// ShellRunner; if the caller doesn't supply one, those expansions return
// an error and no process is spawned.
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
