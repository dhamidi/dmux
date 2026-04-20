package session

import (
	"fmt"
	"time"

	"github.com/dhamidi/dmux/internal/format"
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/layout"
	"github.com/dhamidi/dmux/internal/options"
)

// Server is the root aggregate for all dmux state. It holds every session,
// connected client, paste buffer, option, and hook. Server owns no goroutines
// and performs no I/O; mutations are applied by command handlers (Tier 3).
type Server struct {
	Sessions  map[SessionID]*Session
	Clients   map[ClientID]*Client
	Buffers   *BufferStack
	Options   *options.Store
	Env       Environ
	Hooks     *HookTable
	KeyTables *keys.Registry
	Channels  *ChannelTable
	// Messages holds recent server log lines for show-messages.
	Messages []string
}

// NewServer constructs an empty, ready-to-use Server.
func NewServer() *Server {
	return &Server{
		Sessions:  make(map[SessionID]*Session),
		Clients:   make(map[ClientID]*Client),
		Buffers:   &BufferStack{},
		Options:   options.New(),
		Env:       make(Environ),
		Hooks:     &HookTable{},
		KeyTables: keys.NewRegistry(),
		Channels:  &ChannelTable{},
	}
}

// AddSession inserts s into the server's session table.
func (srv *Server) AddSession(s *Session) {
	srv.Sessions[s.ID] = s
}

// RemoveSession removes the session identified by id. Any clients whose
// Session field points to that session are detached (Session set to nil).
func (srv *Server) RemoveSession(id SessionID) {
	for _, c := range srv.Clients {
		if c.Session != nil && c.Session.ID == id {
			c.Session = nil
		}
	}
	delete(srv.Sessions, id)
}

// AttachClient registers c in the server and attaches it to session sid.
// It returns an error if sid does not name an existing session.
func (srv *Server) AttachClient(c *Client, sid SessionID) error {
	sess, ok := srv.Sessions[sid]
	if !ok {
		return fmt.Errorf("session: session %q not found", sid)
	}
	c.Session = sess
	srv.Clients[c.ID] = c
	return nil
}

// DetachClient removes the client identified by id from its session and from
// the client table. It is a no-op if id is unknown.
func (srv *Server) DetachClient(id ClientID) {
	if c, ok := srv.Clients[id]; ok {
		c.Session = nil
	}
	delete(srv.Clients, id)
}

// Lookup satisfies [format.Context].
// Recognised keys: "session_count".
func (srv *Server) Lookup(key string) (string, bool) {
	switch key {
	case "session_count":
		return fmt.Sprintf("%d", len(srv.Sessions)), true
	}
	return "", false
}

// Children satisfies [format.Context].
// Recognised list keys: "S", "session", "sessions".
func (srv *Server) Children(listKey string) []format.Context {
	switch listKey {
	case "S", "session", "sessions":
		out := make([]format.Context, 0, len(srv.Sessions))
		for _, s := range srv.Sessions {
			out = append(out, s)
		}
		return out
	}
	return nil
}

// Session is one named context of windows. A user may have multiple sessions
// and switch between them. Multiple Winlinks may reference the same *Window
// (the session-group feature).
type Session struct {
	ID           SessionID
	Name         string
	Windows      []*Winlink     // ordered; duplicate Window pointers allowed
	Options      *options.Store // parent = Server.Options
	Env          Environ
	Current      *Winlink
	LastWindowID WindowID // ID of the window that was active before the current one
}

// NewSession creates a Session with the given id and name. The options store
// is a child of parent (which should be Server.Options or nil for a root).
func NewSession(id SessionID, name string, parent *options.Store) *Session {
	return &Session{
		ID:      id,
		Name:    name,
		Options: options.NewChild(parent),
		Env:     make(Environ),
	}
}

// AddWindow appends w to the session's window list as a new Winlink. The
// Winlink index is one greater than the last existing index (1-based). It
// returns the new Winlink. If no current window exists, the new one becomes
// current.
func (s *Session) AddWindow(w *Window) *Winlink {
	idx := 1
	if len(s.Windows) > 0 {
		idx = s.Windows[len(s.Windows)-1].Index + 1
	}
	wl := &Winlink{Index: idx, Window: w}
	s.Windows = append(s.Windows, wl)
	if s.Current == nil {
		s.Current = wl
	}
	return wl
}

// RemoveWindow removes the Winlink at slice position i (0-based). If Current
// pointed to that Winlink, it advances to the next Winlink, then the previous
// one, then nil if the list is now empty.
func (s *Session) RemoveWindow(i int) {
	if i < 0 || i >= len(s.Windows) {
		return
	}
	removed := s.Windows[i]
	s.Windows = append(s.Windows[:i], s.Windows[i+1:]...)
	if s.Current != removed {
		return
	}
	switch {
	case len(s.Windows) == 0:
		s.Current = nil
	case i < len(s.Windows):
		s.Current = s.Windows[i]
	default:
		s.Current = s.Windows[len(s.Windows)-1]
	}
}

// Lookup satisfies [format.Context].
// Recognised keys: "session_id", "session_name", "session_windows".
func (s *Session) Lookup(key string) (string, bool) {
	switch key {
	case "session_id":
		return string(s.ID), true
	case "session_name":
		return s.Name, true
	case "session_windows":
		return fmt.Sprintf("%d", len(s.Windows)), true
	}
	return "", false
}

// Children satisfies [format.Context].
// Recognised list keys: "W", "window", "windows".
func (s *Session) Children(listKey string) []format.Context {
	switch listKey {
	case "W", "window", "windows":
		out := make([]format.Context, len(s.Windows))
		for i, wl := range s.Windows {
			out[i] = wl
		}
		return out
	}
	return nil
}

// Winlink is an entry in a session's window list. It pairs an integer index
// with a *Window. The same *Window may appear at multiple indices within one
// session or across sessions (the session-group feature).
type Winlink struct {
	Index  int
	Window *Window
}

// Lookup satisfies [format.Context]. The key "window_index" returns this
// Winlink's index; all other keys are delegated to the underlying Window.
func (wl *Winlink) Lookup(key string) (string, bool) {
	if key == "window_index" {
		return fmt.Sprintf("%d", wl.Index), true
	}
	return wl.Window.Lookup(key)
}

// Children satisfies [format.Context] by delegating to the underlying Window.
func (wl *Winlink) Children(listKey string) []format.Context {
	return wl.Window.Children(listKey)
}

// Window holds the pane layout and active-pane tracking for one terminal
// window. A Window may be referenced by multiple Sessions via Winlinks.
type Window struct {
	ID               WindowID
	Name             string
	Layout           *layout.Tree
	Panes            map[PaneID]Pane
	Active           PaneID
	LastPaneID       PaneID    // ID of the pane that was active before the current one
	Options          *options.Store
	ActivityFlag     bool      // set when activity/bell detected; cleared on SelectWindow
	LastMonitorCheck time.Time // timestamp of last monitor sweep
	// LastLayout holds the marshalled layout string before the last layout
	// change, enabling select-layout -o (undo).
	LastLayout string
	// CurrentPreset is the name of the last preset applied to this window
	// (e.g. "even-horizontal"). Used to determine the next/previous preset
	// when cycling with select-layout -n/-p.
	CurrentPreset string
}

// NewWindow creates an empty Window with no panes. Call [Window.AddPane] to
// populate it, and assign Layout once the first pane's dimensions are known.
func NewWindow(id WindowID, name string, parent *options.Store) *Window {
	return &Window{
		ID:      id,
		Name:    name,
		Panes:   make(map[PaneID]Pane),
		Options: options.NewChild(parent),
	}
}

// AddPane registers p under the given id. If this is the first pane, Active
// is set to id.
func (w *Window) AddPane(id PaneID, p Pane) {
	w.Panes[id] = p
	if len(w.Panes) == 1 {
		w.Active = id
	}
}

// RemovePane removes the pane identified by id. If Active pointed to that
// pane, Active is reset to zero (the zero value of PaneID / layout.LeafID).
func (w *Window) RemovePane(id PaneID) {
	delete(w.Panes, id)
	if w.Active == id {
		w.Active = 0
	}
}

// Lookup satisfies [format.Context].
// Recognised keys: "window_id", "window_name", "window_panes",
// "window_activity_flag".
func (w *Window) Lookup(key string) (string, bool) {
	switch key {
	case "window_id":
		return string(w.ID), true
	case "window_name":
		return w.Name, true
	case "window_panes":
		return fmt.Sprintf("%d", len(w.Panes)), true
	case "window_activity_flag":
		if w.ActivityFlag {
			return "1", true
		}
		return "0", true
	}
	return "", false
}

// Children satisfies [format.Context]. No list keys are defined for Window yet.
func (w *Window) Children(_ string) []format.Context {
	return nil
}

// Client represents an attached terminal client. Session is nil when the
// client is connected but not yet attached to a session.
type Client struct {
	ID       ClientID
	Session  *Session   // nil when detached
	Size     Size
	TTY      string
	Term     string     // $TERM value reported by the client
	Features FeatureSet
	KeyTable string     // "root" unless inside a prefix sequence or copy-mode
	Overlays []Overlay
	Env      Environ    // environment captured at attach time
	Cwd      string
}

// NewClient creates a Client with sensible defaults. The Session field is nil
// until [Server.AttachClient] is called.
func NewClient(id ClientID) *Client {
	return &Client{
		ID:       id,
		KeyTable: "root",
		Env:      make(Environ),
	}
}

// PushOverlay appends o to the client's overlay stack.
func (c *Client) PushOverlay(o Overlay) {
	c.Overlays = append(c.Overlays, o)
}

// PopOverlay removes and returns the topmost overlay, or nil if the stack is
// empty.
func (c *Client) PopOverlay() Overlay {
	if len(c.Overlays) == 0 {
		return nil
	}
	top := c.Overlays[len(c.Overlays)-1]
	c.Overlays = c.Overlays[:len(c.Overlays)-1]
	return top
}

// Lookup satisfies [format.Context].
// Recognised keys: "client_id", "client_tty", "client_term",
// "client_width", "client_height", "client_key_table", "client_cwd".
func (c *Client) Lookup(key string) (string, bool) {
	switch key {
	case "client_id":
		return string(c.ID), true
	case "client_tty":
		return c.TTY, true
	case "client_term":
		return c.Term, true
	case "client_width":
		return fmt.Sprintf("%d", c.Size.Cols), true
	case "client_height":
		return fmt.Sprintf("%d", c.Size.Rows), true
	case "client_key_table":
		return c.KeyTable, true
	case "client_cwd":
		return c.Cwd, true
	}
	return "", false
}

// Children satisfies [format.Context]. No list keys are defined for Client.
func (c *Client) Children(_ string) []format.Context {
	return nil
}
