// Package parse turns dmux command-language source text into an AST
// of commands and blocks.
//
// # Boundary
//
// Parse(src string) (CommandList, error). That's it. No execution.
//
// The grammar handles:
//   - command and argument tokenization with single/double-quoted strings
//     and backslash escapes
//   - ; separating commands on one line
//   - \ continuing a command across lines
//   - { ... } command blocks (for if-shell, confirm-before, bind-key -T)
//   - if/else blocks
//   - # comments to end-of-line
//
// A CommandList is a slice of Commands, each with a Name string and an
// Args []string. Nested blocks appear as a Command whose body is another
// CommandList.
//
// # I/O surfaces
//
// None. The caller hands in source text as a string; the package
// neither reads files nor opens sockets. The standalone example below
// does the file read itself.
//
// # In isolation
//
// Shipped with a standalone `dmux-parse` example that reads a config
// file and prints the AST as JSON — useful for linting .dmux.conf
// files without booting a server.
//
// # Non-goals
//
// Doesn't know what commands exist. Doesn't validate flags. Doesn't
// expand format strings. Those live in packages command and format.
package parse
