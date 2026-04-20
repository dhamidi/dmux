// Package tree implements the session/window/pane chooser
// (`prefix s`, `choose-tree`, `choose-session`, `choose-window`).
//
// # Data model
//
// The tree mode works exclusively with plain snapshot values — it holds no
// live references to session, window, or pane objects.
//
// [TreeNode] is the fundamental unit:
//
//	type TreeNode struct {
//	    Kind     NodeKind   // KindSession, KindWindow, or KindPane
//	    ID       string     // opaque; passed to the OnSelect callback
//	    Name     string     // human-readable label
//	    Children []TreeNode // nested nodes
//	}
//
// Callers build a []TreeNode slice from whatever source (live server, config
// file, test fixture) and pass it to [New].  The tree mode never mutates the
// slice.
//
// # Boundary
//
// Implements [modes.PaneMode]. Renders a collapsible tree:
//
//	session-a
//	  win-0
//	    pane-0a
//	  win-1
//	session-b
//	  win-2
//
// Supports arrow-key / vim-key navigation (Up/Down/k/j), /-search with
// Backspace editing and Enter/Escape confirmation, and Enter to select.
// q and Escape close the mode without selecting.
//
// # Selection callback
//
// Selection is expressed as a callback rather than direct command dispatch:
//
//	OnSelect func(id string)
//
// When the user presses Enter the callback is invoked with the highlighted
// node's ID, then [modes.CloseMode] is returned.  The caller is responsible
// for translating the ID into whatever command is appropriate
// (e.g. `switch-client -t <target>`).  Passing nil is valid and silently
// skips the callback.
//
// # Preview
//
// When a [PreviewProvider] is supplied, the right half of the mode's area
// shows a [pane.CellGrid] snapshot of the currently-highlighted target.
// [PreviewProvider] is defined as:
//
//	type PreviewProvider func(id string) *pane.CellGrid
//
// Returning nil suppresses the preview for that node.  When the provider
// itself is nil the mode uses the full canvas width for the list.
//
// # In isolation
//
// Construct a []TreeNode slice with synthetic data, call [New] with a stub
// callback and preview provider, then exercise [Mode.Key] and inspect
// [Mode.SelectedID] and the callback result.  No client, session, or real
// terminal is required.
//
// # Non-goals
//
// The tree mode does not import internal/session or any concrete object
// model.  It does not mutate server state.  Command dispatch is delegated
// entirely to the OnSelect callback supplied by the caller.
// This keeps the mode reusable — choose-buffer, choose-client, and
// choose-customize can be implemented in this package as alternative
// constructors over the same tree infrastructure.
package tree
