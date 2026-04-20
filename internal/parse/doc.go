// Package parse turns dmux command-language source text into an AST
// of commands and blocks.
//
// # Boundary
//
// The single public entry point is:
//
//	func Parse(src string) (CommandList, error)
//
// No execution takes place. The package depends only on the Go standard
// library (zero imports of other internal/* packages).
//
// # AST types
//
// CommandList is a []Command.
//
// Command holds:
//
//	Name string       // first token on the logical line
//	Args []string     // remaining tokens
//	Body *CommandList // non-nil when a trailing { ... } block is present
//	Else *CommandList // non-nil when an else { ... } branch follows Body
//
// All fields are plain Go primitives or slices thereof — no external types.
//
// # Grammar
//
// The grammar handles:
//   - command and argument tokenization with single/double-quoted strings
//     and backslash escapes inside double-quoted strings
//   - ; separating commands on one line
//   - \ at end-of-line continuing a command across physical lines
//   - { ... } command blocks (for if-shell, confirm-before, bind-key -T)
//   - if/else blocks: "if-shell … { … } else { … }"
//   - # comments to end-of-line
//
// # In isolation
//
// The package is purely functional: Parse is the only public symbol; there
// is no global state or side-effects beyond returning the AST or an error.
// It can be compiled, tested, and used independently of every other dmux
// package.
//
// Shipped with a standalone dmux-parse example that reads a config file and
// prints the AST as JSON — useful for linting .dmux.conf files without
// booting a server.
//
// # Non-goals
//
// Doesn't know what commands exist. Doesn't validate flags. Doesn't expand
// format strings. Those live in packages command and format.
package parse
