// Package session defines the in-memory model that the server
// manipulates: sessions, windows, winlinks, and their relationships.
//
// # Object graph (matches tmux)
//
//	Session --< Winlink >-- Window --< Pane
//
//	Session   A named group a user attaches to. Holds a list of
//	          Winlinks, a current Winlink, environment, options.
//	Winlink   A Window's entry in one Session, with an index.
//	Window    A tiled group of Panes sharing a size; indirectly
//	          shared between Sessions via Winlinks.
//	Pane      Terminal + child process (see internal/pane).
//
// A Window may appear in multiple Sessions (via separate Winlinks),
// but a Pane belongs to exactly one Window. This matches tmux and
// preserves the linked-session semantics users depend on.
//
// # Scope
//
// Pure data structures plus lookup. This package does not manage
// lifecycles beyond holding pointers; creation, destruction,
// resize orchestration, and alert/activity timers are owned by
// internal/server. Session, Window, and Pane lifetimes are governed
// by reachability from the Registry and from in-flight commands;
// when neither holds them, GC collects.
//
// # Window-size policy (tmux's window-size option)
//
// When multiple clients attach to the same session with different
// terminal sizes, the session must pick one size to use. dmux
// follows tmux's window-size option exactly:
//
//	latest    (default) session sizes to the most recently attached
//	          or most recently resized client
//	largest   session sizes to the largest attached client
//	smallest  session sizes to the smallest attached client
//	manual    size does not track clients; explicit resize-window
//	          only
//
// With `latest`, clients smaller than the session see a letterboxed
// view; clients larger see the session content plus padding. This
// is exactly tmux's behaviour and what users expect.
//
// The session stores its current size and its current
// window-size-policy (read from options when the session is
// created). The server's resize-dispatch code consults the policy
// when a client attaches, resizes, or detaches; the session's size
// may change, which cascades to every pane via vt-first-then-pty
// resize (see internal/pane).
//
// # Active pane, shared per window
//
// A window has exactly one active pane at a time. This state is
// shared across every session that contains the window and every
// client attached to those sessions — matching tmux. When one
// client changes the active pane (select-pane, click-to-focus in
// M3), every other client viewing the same window sees the change
// on the next render.
//
// Rationale: if two users pair on a session, they share a cursor.
// Per-client active-pane state would make split-screen pairing
// confusing: "which pane am I typing into?" becomes ambiguous.
// tmux picked shared; dmux picks shared for the same reason.
//
// Note this is per-window, not per-session. A session whose current
// window changes (via select-window) notifies attached clients, and
// each client's termout re-renders the new window's active pane.
//
// # Implementation note
//
// Lookups are plain `map[ID]*T`. Ordered iteration uses
// `slices.Collect(maps.Values(...))` followed by `slices.SortFunc`
// at the call site. Iterators that produce values one at a time use
// range-over-func (Go 1.23+):
//
//	for p := range win.Panes {
//	    if p.Active { return p }
//	}
//
// tmux uses RB trees for these because C lacks generics; Go has them.
//
// # Interface
//
//	type Registry struct { ... }
//
//	(*Registry) NewSession(name string, opts Options) *Session
//	(*Registry) FindSession(id ID) *Session
//	(*Registry) FindSessionByName(name string) *Session
//	(*Registry) RemoveSession(id ID)
//	(*Registry) Sessions() iter.Seq[*Session]   // ordered by id
//	// similar patterns for Window and Pane
//
// Lookups are plain map accesses. Ordered iteration uses
// range-over-func from Go 1.23+: collect keys, sort once,
// yield in order. Callers compose with slices.Collect when they
// need a slice. This replaces tmux's RB-tree macros, which exist
// only because C lacks generics.
//
// # Concurrency
//
// Registry methods are not safe for concurrent use. They are called
// only from the server's main goroutine (see internal/server). This
// is a hard invariant — adding mutex locking would invite the
// thinking pattern that makes tmux's lifecycle code complex. If you
// find yourself reaching for sync.Mutex here, you are probably
// trying to call a registry method from the wrong goroutine.
//
// # Milestone-one scope
//
// Windows contain exactly one Pane; there is no layout tree, no
// split / join, and no selection of an "active" pane beyond
// "the single pane." Layout work is deferred to a later milestone
// and will extend Window with a layout_root matching tmux's model.
//
// # Corresponding tmux code
//
// session.c and the bookkeeping half of window.c. The other half of
// window.c (pty, input, bufferevent plumbing) is in internal/pane.
package session
