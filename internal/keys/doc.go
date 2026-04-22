// Package keys defines the semantic key event and the data structures
// used to bind keys to commands.
//
// # Event shape
//
// The Event type mirrors libghostty-vt's GhosttyKeyEvent so that the
// pane-side encoder in internal/pane can pass events to
// libghostty-vt's key encoder without translation:
//
//	Action     Press | Release | Repeat
//	Key        layout-independent physical key (W3C UI Events code)
//	Mods       Ctrl / Alt / Shift / Super bitmask, sides preserved
//	Text       UTF-8 text produced by the keypress (may be empty)
//	Unshifted  layout-unmapped Unicode codepoint (for KKP alternate keys)
//	Consumed   modifiers consumed by layout (for KKP)
//	Composing  whether part of an IME composition sequence
//
// # Key codes for bindings
//
// A KeyCode combines a Key with a Mods mask for use as the key in a
// binding map. Not every Event becomes a KeyCode; only Press and
// Repeat actions contribute, with shift normalized for printable keys
// so that "Shift-a" and "A" match the same binding.
//
// # Bindings and tables
//
//	type Binding struct {
//	    Key     KeyCode
//	    CmdList *cmd.List
//	    Note    string
//	    Repeat  bool
//	}
//
//	type Table struct {
//	    Name     string
//	    Bindings map[KeyCode]*Binding
//	}
//
// Tables are named: root, prefix, copy-mode, copy-mode-vi,
// command-prompt, ... matching tmux. Lookup is a single map access.
// There is no hashing of byte sequences; that problem is already
// solved in internal/termin before this package sees anything.
//
// # Defaults
//
// Default bindings are installed by internal/cmd's startup routine
// by parsing bind-key command strings, mirroring tmux's
// key_bindings_init in key-bindings.c. The default binding text does
// not live in this package; this package only defines the types.
package keys
