// Package keys models user-visible keys, key tables, and bindings, and
// decodes keystrokes from the client's real terminal.
//
// # Boundary
//
// Three concerns:
//
//  1. A Key type with a canonical string form ("C-a", "M-x", "Enter",
//     "F1", "C-M-k", "Space", plain runes). Parse(string) (Key, error)
//     and (Key).String() are inverses.
//
//  2. A decoder: Decoder.Next() (Key, error) reads bytes from an
//     io.Reader (the real terminal's stdin) and yields Keys. Handles
//     xterm-style escape sequences, CSI u / Kitty keyboard protocol,
//     bracketed paste, and mouse sequences.
//
//  3. A named table registry: Table holds Key → Binding, and Registry
//     holds Name → *Table. A Binding is an opaque value provided by
//     the caller (in practice, a command.Command), so this package
//     does not import command.
//
// # Nested bindings
//
// Each attached client has a "current table" name (see session.Client).
// Dispatch logic lives in the server loop: look up the client's table,
// find the binding for the key, enqueue it. Tables like `prefix`,
// `copy-mode-vi`, and any user-defined `bind-key -T foo` are registered
// here but changed by commands.
//
// # In isolation
//
// Ships a REPL example that prints the decoded Key for each keypress.
// Useful for debugging terminfo weirdness.
//
// # Non-goals
//
// Does NOT encode keys to be sent to panes' shells. That's go-libghostty's
// KeyEncoder, invoked by package pane.
package keys
