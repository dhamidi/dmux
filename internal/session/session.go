package session

import (
	"cmp"
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"

	"github.com/dhamidi/dmux/internal/options"
	"github.com/dhamidi/dmux/internal/pane"
)

// Sentinel errors. Callers dispatch on category via errors.Is;
// structured detail rides on *Error wrapping one of these.
var (
	// ErrNoSuchSession indicates a Find by id or name found nothing.
	// Command-family callers map it to cmd.ErrNotFound at their own
	// boundary; the session package uses its own sentinel so internal
	// call sites can tell "no session here" from other not-found
	// categories.
	ErrNoSuchSession = errors.New("session: no such session")

	// ErrDuplicateSession is returned by Registry.NewSession when a
	// session with the requested name already exists. The server's
	// default flow never hits this because it always asks for
	// name="dmux" on a fresh registry.
	ErrDuplicateSession = errors.New("session: duplicate name")
)

// Error carries structured context for failures from this package.
// The Sentinel field is always one of the package sentinels, so
// errors.Is on the sentinel works regardless of whether the caller
// holds the concrete type. Name is the session name involved when
// known; ID is the numeric id when known. Err is a wrapped cause.
type Error struct {
	Op       string // what was being attempted ("find", "new", etc.)
	Sentinel error  // one of the package sentinels
	Name     string // session name, when relevant
	ID       ID     // session id, when relevant
	Err      error  // underlying cause, or nil
}

// Error renders the failure as lowercase, no-trailing-punctuation
// text. The chain is "session: <op>: <sentinel-tail> [name=<n>] [id=<i>] [: cause]".
func (e *Error) Error() string {
	out := "session: " + e.Op
	if e.Sentinel != nil {
		// Strip the "session: " prefix from the sentinel text to
		// avoid repeating it in the composed chain.
		tail := e.Sentinel.Error()
		const prefix = "session: "
		if len(tail) >= len(prefix) && tail[:len(prefix)] == prefix {
			tail = tail[len(prefix):]
		}
		out += ": " + tail
	}
	if e.Name != "" {
		out += fmt.Sprintf(" name=%q", e.Name)
	}
	if e.ID != 0 {
		out += fmt.Sprintf(" id=%d", e.ID)
	}
	if e.Err != nil {
		out += ": " + e.Err.Error()
	}
	return out
}

// Unwrap returns the underlying cause, preserving errors.Is/As
// traversal through the chain.
func (e *Error) Unwrap() error { return e.Err }

// Is matches the package sentinel even when Err carries an unrelated
// chain. This mirrors the pane package's Is implementation: the
// category is a property of Error, not of its cause.
func (e *Error) Is(target error) bool {
	return e.Sentinel != nil && e.Sentinel == target
}

// ID uniquely identifies a session for this process's lifetime. IDs
// are monotonically increasing uint64 starting at 1; zero is reserved
// for "no session."
type ID uint64

// Registry owns every live session in this server. Lookups are plain
// map accesses; ordered iteration collects and sorts at call time.
//
// Registry methods are NOT safe for concurrent use. They are called
// only from the server's main goroutine. Adding a mutex here would
// invite the thinking pattern that makes tmux's lifecycle code hard
// to reason about; if you feel the need, the caller is on the wrong
// goroutine.
type Registry struct {
	byID   map[ID]*Session
	byName map[string]*Session
	nextID ID
}

// NewRegistry returns an empty Registry ready to accept sessions.
func NewRegistry() *Registry {
	return &Registry{
		byID:   make(map[ID]*Session),
		byName: make(map[string]*Session),
		nextID: 0,
	}
}

// NewSession creates a fresh session named name and registers it with
// a session-scoped Options child parented at parentOpts. Returns an
// error wrapping ErrDuplicateSession if a session with this name
// already exists. The returned session has no windows yet — call
// AppendWindow to populate one.
//
// parentOpts is typically the server's options so Get on the session
// walks server-scoped overrides before falling through to Table
// defaults. Pass nil from tests that do not exercise option lookup;
// the session will still have its own empty session-scoped Options.
func (r *Registry) NewSession(name string, parentOpts *options.Options) (*Session, error) {
	if _, exists := r.byName[name]; exists {
		return nil, &Error{
			Op:       "new",
			Sentinel: ErrDuplicateSession,
			Name:     name,
		}
	}
	r.nextID++
	s := &Session{
		id:         r.nextID,
		name:       name,
		currentIdx: -1,
		options:    options.NewScopedOptions(options.SessionScope, parentOpts),
	}
	r.byID[s.id] = s
	r.byName[name] = s
	return s, nil
}

// FindSession returns the session with the given id, or nil if none
// is registered. Callers that want a typed error rather than a nil
// check can wrap the nil return themselves; the registry exposes nil
// because the zero-allocation path matters here (this is called on
// every command that accepts a -t target).
func (r *Registry) FindSession(id ID) *Session {
	return r.byID[id]
}

// FindSessionByName returns the session registered under name, or nil
// if no such session exists.
func (r *Registry) FindSessionByName(name string) *Session {
	return r.byName[name]
}

// RemoveSession unregisters the session with the given id. No-op if
// no session with that id exists. The session's windows and their
// panes are left untouched; the caller is responsible for closing any
// panes before calling this.
func (r *Registry) RemoveSession(id ID) {
	s, ok := r.byID[id]
	if !ok {
		return
	}
	delete(r.byID, id)
	delete(r.byName, s.name)
}

// Sessions returns a range-over-func iterator over every session,
// ordered by id ascending. The iteration snapshot is taken at call
// time (maps.Values + slices.SortFunc), so concurrent mutation during
// iteration is safe in the same sense the rest of the package is:
// don't do it from another goroutine; mutating from inside the loop
// is fine.
func (r *Registry) Sessions() iter.Seq[*Session] {
	ordered := slices.Collect(maps.Values(r.byID))
	slices.SortFunc(ordered, func(a, b *Session) int {
		return cmp.Compare(a.id, b.id)
	})
	return func(yield func(*Session) bool) {
		for _, s := range ordered {
			if !yield(s) {
				return
			}
		}
	}
}

// Len reports the total number of registered sessions. Used by the
// server's HasSession gate to decide whether attach-session should
// return ErrNotFound.
func (r *Registry) Len() int { return len(r.byID) }

// Session is one named session. It owns an ordered slice of windows
// (creation order) and an index into that slice identifying the
// current window. The current-index cursor is -1 on a fresh session
// with no windows and advances to the tail as windows are appended.
//
// Once appended, a Window's Index never changes — even if earlier
// windows are later closed. This matches tmux's behavior
// (select-window -t :3 always hits window 3 regardless of intervening
// closes). M1 does not implement window-close; the stability
// invariant is future-proofing for M2.
type Session struct {
	id         ID
	name       string
	windows    []*Window
	currentIdx int // -1 when windows is empty
	nextWinIdx int // monotonic counter for Window.index assignment
	options    *options.Options
}

// ID returns this session's registry id.
func (s *Session) ID() ID { return s.id }

// Name returns this session's registered name.
func (s *Session) Name() string { return s.name }

// Options returns the session's session-scoped Options. Guaranteed
// non-nil for sessions created through Registry.NewSession.
func (s *Session) Options() *options.Options { return s.options }

// CurrentWindow returns the session's current window, or nil if the
// session has no windows. Navigation commands (next-window,
// previous-window) advance the underlying current-index cursor; a
// fresh AppendWindow also sets the new window as current.
func (s *Session) CurrentWindow() *Window {
	if s.currentIdx < 0 || s.currentIdx >= len(s.windows) {
		return nil
	}
	return s.windows[s.currentIdx]
}

// Windows returns a snapshot of the session's windows in creation
// order. The returned slice is a copy; callers may retain or mutate
// it without affecting the session. Used by diagnostic callers
// (list-windows); navigation commands use the CurrentWindow /
// NextWindow / PreviousWindow cursor instead.
func (s *Session) Windows() []*Window {
	return append([]*Window(nil), s.windows...)
}

// AppendWindow creates a fresh window named name, appends it to the
// session's window list, and sets it as the current window. The new
// window's Index is its position at append time (len(windows) before
// the append). Never errors in M1: there is no duplicate-name
// constraint across windows, and every append succeeds.
//
// Callers that need to wire an active pane into the new window do so
// via (*Window).SetActivePane after AppendWindow returns.
func (s *Session) AppendWindow(name string) (*Window, error) {
	w := &Window{
		index: s.nextWinIdx,
		name:  name,
	}
	s.nextWinIdx++
	s.windows = append(s.windows, w)
	s.currentIdx = len(s.windows) - 1
	return w, nil
}

// RemoveWindow removes w from the session's window list. Returns true
// if w was present and removed, false otherwise. When the removed
// window was the current one, the cursor advances to the window that
// was at the next slice position (or wraps to 0 if the removed window
// was the last). When the removed window sat before the cursor, the
// cursor shifts down by one so it still points at the same Window.
// When the last remaining window is removed, currentIdx returns to -1
// and CurrentWindow reports nil.
//
// Window.Index values of surviving windows never change: they were
// assigned from a monotonic counter at AppendWindow time, and
// RemoveWindow does not renumber.
func (s *Session) RemoveWindow(w *Window) bool {
	pos := -1
	for i, cand := range s.windows {
		if cand == w {
			pos = i
			break
		}
	}
	if pos < 0 {
		return false
	}
	s.windows = append(s.windows[:pos], s.windows[pos+1:]...)
	switch {
	case len(s.windows) == 0:
		s.currentIdx = -1
	case pos < s.currentIdx:
		s.currentIdx--
	case pos == s.currentIdx:
		if s.currentIdx >= len(s.windows) {
			s.currentIdx = 0
		}
	}
	return true
}

// NextWindow advances the current-window cursor by one, wrapping
// from the last window back to the first. Returns the new current
// window, or nil if the session has no windows. A single-window
// session is a no-op: NextWindow returns the sole window with the
// cursor unchanged.
func (s *Session) NextWindow() *Window {
	if len(s.windows) == 0 {
		return nil
	}
	s.currentIdx = (s.currentIdx + 1) % len(s.windows)
	return s.windows[s.currentIdx]
}

// PreviousWindow rewinds the current-window cursor by one, wrapping
// from the first window back to the last. Returns the new current
// window, or nil if the session has no windows. A single-window
// session is a no-op: PreviousWindow returns the sole window with
// the cursor unchanged.
func (s *Session) PreviousWindow() *Window {
	if len(s.windows) == 0 {
		return nil
	}
	s.currentIdx = (s.currentIdx - 1 + len(s.windows)) % len(s.windows)
	return s.windows[s.currentIdx]
}

// Window holds the active pane for one tiled group. M1: exactly one
// pane per window; "active pane" is just "the pane."
type Window struct {
	index  int
	name   string
	active *pane.Pane
}

// Index returns the window's index within its session. Stable for
// the lifetime of the window: never renumbered when earlier windows
// close.
func (w *Window) Index() int { return w.index }

// Name returns the window's name. Typically derived from the pane's
// argv[0] (e.g. "bash", "zsh"), but the session package treats it as
// opaque text.
func (w *Window) Name() string { return w.name }

// ActivePane returns the pane currently displayed in this window, or
// nil if SetActivePane has not yet been called.
func (w *Window) ActivePane() *pane.Pane { return w.active }

// SetActivePane records p as this window's active pane. M1 callers
// set this exactly once, right after pane.Open. M2 select-pane will
// take over this setter.
func (w *Window) SetActivePane(p *pane.Pane) { w.active = p }
