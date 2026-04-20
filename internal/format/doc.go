// Package format expands #{...} format strings.
//
// # Public API
//
// The primary entry point is [Expand]:
//
//	result, err := format.Expand(template, ctx)
//
// For templates that use #(...) shell command directives, create an [Expander]
// with a [CommandRunner]:
//
//	e := format.New(runner)
//	result, err := e.Expand(template, ctx)
//
// # Context interface
//
// [Context] supplies variable values for expansion:
//
//	type Context interface {
//	    Lookup(key string) (string, bool)
//	    Children(listKey string) []Context
//	}
//
// [MapContext] is a ready-made implementation backed by map[string]string.
// Because Context is an interface, package session provides bindings when
// expanding for a status line, but format itself has no dependency on session
// and can be used with a plain map for testing or general templating.
//
// # CommandRunner interface
//
// [CommandRunner] executes external commands for #(...) directives:
//
//	type CommandRunner interface {
//	    Run(name string, args []string) (output string, err error)
//	}
//
// The format package never spawns processes; callers supply a CommandRunner
// implementation (e.g. one wrapping package job). Passing nil to [New]
// disables shell command expansion — any #(...) directive expands to "".
//
// # Template syntax
//
// The syntax mirrors tmux format strings:
//   - #{var}                  simple variable substitution
//   - #{?cond,yes,no}         conditional; cond may be a variable name or a
//     nested #{...} expression; yes/no are recursively expanded
//   - #{=N:var}               truncate to first N runes (N < 0: last N runes)
//   - #{s/pat/repl/:var}      regex substitution (all occurrences)
//   - #{||:a,b}               "1" if either a or b is truthy, "0" otherwise
//   - #{&&:a,b}               "1" if both a and b are truthy, "0" otherwise
//   - #{T:var}                interpret var as Unix timestamp; format as
//     "2006-01-02 15:04:05" (UTC)
//   - #{t:var}                interpret var as Unix timestamp; format as
//     relative time (e.g. "5m", "2h", "3d")
//   - #(cmd arg…)             run cmd via CommandRunner; output replaces the
//     directive (trailing newline stripped)
//
// Truth: a value is truthy when it is non-empty and not "0".
//
// # In isolation
//
// Reusable as a general templating library. No process spawning, no session
// state, no I/O required — all behaviour is provided through the two
// interfaces above.
//
// # Non-goals
//
// No rendering (produces plain strings; package status turns them into cells).
// No caching of expensive expansions — callers that care cache their own results.
package format
