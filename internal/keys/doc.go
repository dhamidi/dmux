// Package keys models user-visible keys, key tables, and bindings, and
// decodes keystrokes from the client's real terminal.
//
// # Key model
//
// A [Key] carries three fields:
//
//   - Code  – a [KeyCode] identifying the key. Positive values are
//     Unicode code points (e.g. 'a', 'A', '€'). Negative values are
//     named constants for special keys (see CodeEnter, CodeEscape,
//     CodeUp, CodeF1, …). CodeNone (0) is the zero/invalid value.
//
//   - Mod   – a [Modifier] bitmask of [ModCtrl], [ModAlt]/Meta,
//     and [ModShift].
//
//   - Mouse – a [MouseEvent] carrying button, action, and position;
//     only meaningful when Code == [CodeMouse].
//
// The canonical string form follows tmux notation:
//
//	C-a     Ctrl+a
//	M-x     Alt/Meta+x
//	C-M-k   Ctrl+Alt+k
//	Enter   Enter key
//	F1      Function key 1
//	Space   Space character
//	a       literal 'a'
//
// [Parse] and [Key.String] are exact inverses for all valid keys.
//
// # Decoder
//
// [Decoder] wraps an [io.Reader] (the real terminal's stdin) and
// yields Key events via [Decoder.Next]. It is a pure function of the
// input bytes: it has no side effects other than advancing the read
// position and returning a Key value.
//
// Supported input protocols:
//
//   - Printable Unicode characters and ASCII control codes
//   - xterm-style escape sequences (CSI cursor/nav/function keys,
//     including modifier parameters: ESC[1;5A = Ctrl+Up)
//   - SS3 sequences (ESC O P = F1, ESC O A = Up, …)
//   - CSI u keyboard protocol (ESC[codepoint;mods u)
//   - Kitty keyboard protocol (superset of CSI u with event type:
//     ESC[codepoint;mods:eventtype u)
//   - Bracketed paste (ESC[200~ yields [CodePasteStart]; ESC[201~
//     yields [CodePasteEnd]; paste text arrives as ordinary keys
//     between them)
//   - SGR mouse sequences (ESC[<btn;col;rowM/m)
//   - X10 mouse sequences (ESC[M + 3 raw bytes)
//
// # Bound commands
//
// [BoundCommand] is defined as any, keeping this package independent
// of internal/command. Callers store a raw command string (the text
// to be parsed and dispatched by the server loop) as the bound value.
//
// # Key binding registry
//
// [Table] maps [Key] → [BoundCommand]. Each named key table
// (e.g. "root", "prefix", "copy-mode-vi") is one Table.
//
// [Registry] maps table name → *[Table]. The server loop holds one
// Registry; each attached client tracks its current table name
// (see package session). Dispatch: look up the client's current
// table, call [Table.Lookup] with the decoded key, enqueue the
// resulting command. All dispatch logic lives outside this package.
//
// # In isolation
//
// Ships a REPL example that prints the decoded Key for each keypress.
// Useful for debugging terminfo weirdness.
//
// # Non-goals
//
// Does NOT encode keys to be sent to panes' shells. That's
// go-libghostty's KeyEncoder, invoked by package pane.
package keys
