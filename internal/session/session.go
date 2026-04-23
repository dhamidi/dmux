package session

import (
	"cmp"
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"

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
	// session with the requested name already exists. M1's policy is
	// fail-fast on duplicate names; the server's default flow never
	// hits this because it always asks for name="dmux" on a fresh
	// registry.
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

// NewSession creates a fresh session named name and registers it.
// Returns an error wrapping ErrDuplicateSession if a session with
// this name already exists. The returned session has no windows yet
// — call AddWindow to populate one.
func (r *Registry) NewSession(name string) (*Session, error) {
	if _, exists := r.byName[name]; exists {
		return nil, &Error{
			Op:       "new",
			Sentinel: ErrDuplicateSession,
			Name:     name,
		}
	}
	r.nextID++
	s := &Session{
		id:   r.nextID,
		name: name,
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

// Session is one named session. M1 scope: a single window attached
// directly (no Winlink indirection). The window is the implicit
// "current window"; its sole pane is the implicit "active pane."
type Session struct {
	id     ID
	name   string
	window *Window // M1: at most one
}

// ID returns this session's registry id.
func (s *Session) ID() ID { return s.id }

// Name returns this session's registered name.
func (s *Session) Name() string { return s.name }

// CurrentWindow returns the session's current window, or nil if no
// window has been added yet. M1 only ever has one window; the
// "current" distinction becomes real when select-window lands.
func (s *Session) CurrentWindow() *Window { return s.window }

// AddWindow creates and attaches a fresh window named name. M1
// permits at most one window per session; calling AddWindow twice
// returns an error wrapping ErrDuplicateSession — the nearest-fit
// sentinel until a dedicated ErrDuplicateWindow lands in M2.
func (s *Session) AddWindow(name string) (*Window, error) {
	if s.window != nil {
		return nil, &Error{
			Op:       "add-window",
			Sentinel: ErrDuplicateSession,
			Name:     name,
			ID:       s.id,
		}
	}
	w := &Window{
		index: 0,
		name:  name,
	}
	s.window = w
	return w, nil
}

// Window holds the active pane for one tiled group. M1: exactly one
// pane per window; "active pane" is just "the pane."
type Window struct {
	index  int
	name   string
	active *pane.Pane
}

// Index returns the window's index within its session. M1: always 0.
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
