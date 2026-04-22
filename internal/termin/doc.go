// Package termin parses the byte stream coming from a client's real
// terminal into semantic input events.
//
// It is the inverse of libghostty-vt's key / mouse / focus encoders.
// libghostty-vt turns structured events into bytes destined for a pty;
// termin turns bytes coming the other way (from the user's terminal)
// back into structured events.
//
// Libghostty-vt does not provide this direction, so this is the one
// input parser dmux owns.
//
// # Events emitted
//
//	keys.Event       press/release/repeat with mods and UTF-8 text
//	MouseEvent       SGR-encoded mouse with cell coordinates
//	FocusEvent       in/out
//	PasteEvent       a complete bracketed paste payload
//	CapsResponse     deferred DA/DSR/KKP replies observed on stdin
//
// # Parser profiles
//
// A Parser is constructed for one of the termcaps.Profile values.
// This selects which key-encoding dialect is active:
//
//   - KKP (Ghostty, XTermJSModern)
//   - modifyOtherKeys + legacy (XTermJSLegacy, WindowsTerminal, Unknown)
//
// SGR mouse, bracketed paste, and focus in/out are always on; all
// three targets support them, so no fallback is needed.
//
// The parser does not auto-detect mid-stream. If the profile changes
// (e.g. via a CapsUpdate after a late DA2 reply), the server
// reconstructs the Parser.
//
// # Interface
//
//	NewParser(termcaps.Profile) *Parser
//	(*Parser) Feed(b []byte) []Event
//	(*Parser) Tick(now time.Time) []Event   // fires pending escape-timeout events
//	(*Parser) Reset()
//
// ESC-timeout handling (distinguishing a lone Escape from the start
// of an escape sequence) requires a timer. The parser reports its
// pending deadline; the server's per-client loop drives Tick. The
// parser itself has no clock.
//
// # Scope boundary
//
// termin does not know about key tables, bindings, commands, or panes.
// It converts bytes to events; routing is internal/server, matching
// is internal/keys, dispatch is internal/cmdq, and pane-side
// re-encoding is internal/pane.
//
// # Corresponding tmux code
//
// tmux's tty-keys.c, but narrower due to the closed target list: no
// terminfo-derived key tables, no URxvt dialect, no DEC soft-function
// keys, no legacy X10 mouse.
package termin
