package session

import (
	"fmt"
	"os"
	"strings"
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
	// ACL is the access control list for server connections.
	// Maps Unix username to allowed (true) or denied (false).
	ACL map[string]bool
	// ACLWriteAccess maps usernames that have been granted write access.
	ACLWriteAccess map[string]bool
	// ACLDenyAll blocks all new connections when true.
	ACLDenyAll bool
	// StartTime is when the server process was started.
	StartTime time.Time
	// SocketPath is the filesystem path of the listening Unix-domain socket.
	SocketPath string
	// PidValue is the OS process ID of the server (populated by NewServer).
	PidValue int
	// Version is the server version string (e.g. "dmux 0.1").
	Version string
}

// NewServer constructs an empty, ready-to-use Server.
func NewServer() *Server {
	return &Server{
		Sessions:       make(map[SessionID]*Session),
		Clients:        make(map[ClientID]*Client),
		Buffers:        &BufferStack{},
		Options:        options.New(),
		Env:            make(Environ),
		Hooks:          &HookTable{},
		KeyTables:      keys.NewRegistry(),
		Channels:       &ChannelTable{},
		ACL:            make(map[string]bool),
		ACLWriteAccess: make(map[string]bool),
		StartTime:      time.Now(),
		PidValue:       os.Getpid(),
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
	sess.Attached++
	sess.AttachedList = append(sess.AttachedList, c.ID)
	sess.LastAttachedAt = time.Now()
	return nil
}

// DetachClient removes the client identified by id from its session and from
// the client table. It is a no-op if id is unknown.
func (srv *Server) DetachClient(id ClientID) {
	if c, ok := srv.Clients[id]; ok {
		if c.Session != nil {
			c.Session.Attached--
			for i, cid := range c.Session.AttachedList {
				if cid == id {
					c.Session.AttachedList = append(c.Session.AttachedList[:i], c.Session.AttachedList[i+1:]...)
					break
				}
			}
		}
		c.Session = nil
	}
	delete(srv.Clients, id)
}

// Lookup satisfies [format.Context].
// Recognised keys: "session_count", "pid", "host", "host_short",
// "socket_path", "start_time", "version".
func (srv *Server) Lookup(key string) (string, bool) {
	switch key {
	case "session_count":
		return fmt.Sprintf("%d", len(srv.Sessions)), true
	case "pid":
		return fmt.Sprintf("%d", srv.PidValue), true
	case "host":
		h, _ := os.Hostname()
		return h, true
	case "host_short":
		h, _ := os.Hostname()
		if i := strings.Index(h, "."); i >= 0 {
			h = h[:i]
		}
		return h, true
	case "socket_path":
		return srv.SocketPath, true
	case "start_time":
		return fmt.Sprintf("%d", srv.StartTime.Unix()), true
	case "version":
		if srv.Version != "" {
			return srv.Version, true
		}
		return "dmux", true
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
	// CreatedAt is when this session was created.
	CreatedAt time.Time
	// LastAttachedAt is when a client last attached to this session.
	LastAttachedAt time.Time
	// Path is the default working directory for this session.
	Path string
	// Attached is the number of currently attached clients.
	Attached int
	// AttachedList holds the IDs of currently attached clients.
	AttachedList []ClientID
}

// NewSession creates a Session with the given id and name. The options store
// is a child of parent (which should be Server.Options or nil for a root).
func NewSession(id SessionID, name string, parent *options.Store) *Session {
	return &Session{
		ID:        id,
		Name:      name,
		Options:   options.NewChild(parent),
		Env:       make(Environ),
		CreatedAt: time.Now(),
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
	wl := &Winlink{Index: idx, Window: w, Session: s}
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
// Recognised keys: all session_* format variables.
func (s *Session) Lookup(key string) (string, bool) {
	switch key {
	case "session_id":
		return string(s.ID), true
	case "session_name":
		return s.Name, true
	case "session_windows":
		return fmt.Sprintf("%d", len(s.Windows)), true
	case "session_activity":
		ts := s.LastAttachedAt
		if ts.IsZero() {
			ts = s.CreatedAt
		}
		return fmt.Sprintf("%d", ts.Unix()), true
	case "session_alerts":
		return "", true
	case "session_attached":
		return fmt.Sprintf("%d", s.Attached), true
	case "session_attached_list":
		ids := make([]string, len(s.AttachedList))
		for i, id := range s.AttachedList {
			ids[i] = string(id)
		}
		return strings.Join(ids, ","), true
	case "session_created":
		return fmt.Sprintf("%d", s.CreatedAt.Unix()), true
	case "session_format":
		return "1", true
	case "session_group":
		// Sessions are not grouped by default; return empty.
		return "", true
	case "session_group_attached":
		return fmt.Sprintf("%d", s.Attached), true
	case "session_group_attached_list":
		ids := make([]string, len(s.AttachedList))
		for i, id := range s.AttachedList {
			ids[i] = string(id)
		}
		return strings.Join(ids, ","), true
	case "session_group_list":
		return string(s.ID), true
	case "session_group_many_attached":
		if s.Attached > 1 {
			return "1", true
		}
		return "0", true
	case "session_group_size":
		return "1", true
	case "session_grouped":
		return "0", true
	case "session_last_attached":
		if !s.LastAttachedAt.IsZero() {
			return fmt.Sprintf("%d", s.LastAttachedAt.Unix()), true
		}
		return fmt.Sprintf("%d", s.CreatedAt.Unix()), true
	case "session_many_attached":
		if s.Attached > 1 {
			return "1", true
		}
		return "0", true
	case "session_marked":
		return "0", true
	case "session_path":
		return s.Path, true
	case "session_stack":
		return "", true
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
	Index   int
	Window  *Window
	Session *Session // back-reference to the owning session; set by AddWindow
}

// Lookup satisfies [format.Context]. Session-position variables
// (window_active, window_start_flag, window_end_flag, window_last_flag,
// window_flags) are resolved here; window_index returns this Winlink's index;
// all other keys are delegated to the underlying Window.
func (wl *Winlink) Lookup(key string) (string, bool) {
	switch key {
	case "window_index":
		return fmt.Sprintf("%d", wl.Index), true
	case "window_active":
		if wl.Session != nil && wl.Session.Current == wl {
			return "1", true
		}
		return "0", true
	case "window_last_flag":
		if wl.Session != nil && wl.Window.ID == wl.Session.LastWindowID {
			return "1", true
		}
		return "0", true
	case "window_start_flag":
		if wl.Session != nil && len(wl.Session.Windows) > 0 && wl.Session.Windows[0] == wl {
			return "1", true
		}
		return "0", true
	case "window_end_flag":
		if wl.Session != nil && len(wl.Session.Windows) > 0 && wl.Session.Windows[len(wl.Session.Windows)-1] == wl {
			return "1", true
		}
		return "0", true
	case "window_flags":
		var flags []byte
		if wl.Session != nil {
			if wl.Session.Current == wl {
				flags = append(flags, '*')
			}
			if wl.Window.ID == wl.Session.LastWindowID {
				flags = append(flags, '-')
			}
		}
		if wl.Window.ActivityFlag {
			flags = append(flags, '!')
		}
		if len(flags) == 0 {
			return " ", true
		}
		return string(flags), true
	case "window_stack_index":
		if wl.Session != nil {
			for i, w := range wl.Session.Windows {
				if w == wl {
					return fmt.Sprintf("%d", i), true
				}
			}
		}
		return "0", true
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
	// LinkedSessions records every session this window is linked into via
	// link-window. It is empty for windows that have never been linked.
	// When a window is linked into a second session both the originating
	// session and the destination session appear in this slice.
	LinkedSessions []SessionID
	// ActivityAt is when activity was last detected in this window (bell/content change).
	ActivityAt time.Time
	// PaneTTYs maps each pane ID to its PTY device path (e.g. "/dev/pts/3").
	// Populated by the server when creating panes.
	PaneTTYs map[PaneID]string
	// PaneStartCmds maps each pane ID to the command it was started with.
	PaneStartCmds map[PaneID]string
	// PaneStartPaths maps each pane ID to the working directory when it was created.
	PaneStartPaths map[PaneID]string
}

// NewWindow creates an empty Window with no panes. Call [Window.AddPane] to
// populate it, and assign Layout once the first pane's dimensions are known.
func NewWindow(id WindowID, name string, parent *options.Store) *Window {
	return &Window{
		ID:             id,
		Name:           name,
		Panes:          make(map[PaneID]Pane),
		Options:        options.NewChild(parent),
		PaneTTYs:       make(map[PaneID]string),
		PaneStartCmds:  make(map[PaneID]string),
		PaneStartPaths: make(map[PaneID]string),
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

// AddLinkedSession records that this window also appears in sess. It is
// idempotent: adding the same session twice has no effect.
func (w *Window) AddLinkedSession(id SessionID) {
	for _, s := range w.LinkedSessions {
		if s == id {
			return
		}
	}
	w.LinkedSessions = append(w.LinkedSessions, id)
}

// RemoveLinkedSession removes sess from the linked-sessions list. It is a
// no-op if sess is not present.
func (w *Window) RemoveLinkedSession(id SessionID) {
	for i, s := range w.LinkedSessions {
		if s == id {
			w.LinkedSessions = append(w.LinkedSessions[:i], w.LinkedSessions[i+1:]...)
			return
		}
	}
}

// boolVal returns "1" if b is true, "0" otherwise. Used for boolean format variables.
func boolVal(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// Lookup satisfies [format.Context].
// Recognised keys: all window_* format variables.
func (w *Window) Lookup(key string) (string, bool) {
	switch key {
	case "window_id":
		return string(w.ID), true
	case "window_name":
		return w.Name, true
	case "window_panes":
		return fmt.Sprintf("%d", len(w.Panes)), true
	case "window_activity_flag", "window_bell_flag":
		return boolVal(w.ActivityFlag), true
	case "window_activity":
		return fmt.Sprintf("%d", w.ActivityAt.Unix()), true
	case "window_active":
		// Without session context, we can't determine this; return 0.
		// The correct value is computed in Winlink.Lookup.
		return "0", true
	case "window_bigger":
		return "0", true
	case "window_cell_height", "window_cell_width":
		return "0", true
	case "window_end_flag", "window_start_flag":
		// Correct values are computed in Winlink.Lookup.
		return "0", true
	case "window_flags":
		// Flags without session context: only show activity flag.
		if w.ActivityFlag {
			return "!", true
		}
		return " ", true
	case "window_format":
		return "1", true
	case "window_height":
		if w.Layout != nil {
			return fmt.Sprintf("%d", w.Layout.Rows()), true
		}
		return "0", true
	case "window_last_flag":
		// Correct value is computed in Winlink.Lookup.
		return "0", true
	case "window_layout":
		if w.Layout != nil {
			return w.Layout.Marshal(), true
		}
		return "0x0,0,0,0", true
	case "window_linked":
		return boolVal(len(w.LinkedSessions) > 1), true
	case "window_linked_sessions":
		return fmt.Sprintf("%d", len(w.LinkedSessions)), true
	case "window_linked_sessions_list":
		ids := make([]string, len(w.LinkedSessions))
		for i, id := range w.LinkedSessions {
			ids[i] = string(id)
		}
		return strings.Join(ids, ","), true
	case "window_marked_flag":
		return "0", true
	case "window_offset_x", "window_offset_y":
		return "0", true
	case "window_raw_flags":
		if w.ActivityFlag {
			return "!", true
		}
		return "", true
	case "window_silence_flag":
		return "0", true
	case "window_stack_index":
		// Correct value is computed in Winlink.Lookup.
		return "0", true
	case "window_visible_layout":
		if w.Layout != nil {
			return w.Layout.Marshal(), true
		}
		return "0x0,0,0,0", true
	case "window_width":
		if w.Layout != nil {
			return fmt.Sprintf("%d", w.Layout.Cols()), true
		}
		return "0", true
	case "window_zoomed_flag":
		if w.Layout != nil && w.Layout.IsZoomed() {
			return "1", true
		}
		return "0", true
	}
	return "", false
}

// Children satisfies [format.Context].
// Recognised list keys: "P", "pane", "panes" — returns one PaneContext per pane.
func (w *Window) Children(listKey string) []format.Context {
	switch listKey {
	case "P", "pane", "panes":
		return w.buildPaneContexts()
	}
	return nil
}

// buildPaneContexts constructs a PaneContext for each pane in the window.
func (w *Window) buildPaneContexts() []format.Context {
	if w.Layout == nil {
		return nil
	}
	winW := w.Layout.Cols()
	winH := w.Layout.Rows()
	out := make([]format.Context, 0, len(w.Panes))
	idx := 0
	for id, p := range w.Panes {
		r := w.Layout.Rect(id)
		pc := &PaneContext{
			PaneID:       id,
			PaneIndex:    idx,
			Left:         r.X,
			Top:          r.Y,
			Width:        r.Width,
			Height:       r.Height,
			WindowWidth:  winW,
			WindowHeight: winH,
			Active:       w.Active == id,
			Last:         w.LastPaneID == id,
			Title:        p.Title(),
			ShellPID:     p.ShellPID(),
			TTY:          w.PaneTTYs[id],
			StartCommand: w.PaneStartCmds[id],
			StartPath:    w.PaneStartPaths[id],
		}
		if w.Layout.IsZoomed() {
			pc.Zoomed = w.Layout.ZoomedLeaf() == id
		}
		out = append(out, pc)
		idx++
	}
	return out
}

// Subscription is a named notification subscription registered via
// refresh-client -B name:notify:format.
type Subscription struct {
	Name   string
	Notify string
	Format string
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
	// PID is the OS process ID of the dmux client process. Used by
	// suspend-client to send SIGTSTP to the client.
	PID int
	// CreatedAt is when this client connected.
	CreatedAt time.Time
	// UID is the Unix user ID of the connecting client.
	UID int
	// UserName is the username of the connecting client.
	UserName string
	// LastSession is the name of the session this client was last attached to.
	LastSession string
	// Prefix indicates the client is currently in the prefix key state.
	Prefix bool
	// Readonly indicates the client was attached in read-only mode.
	Readonly bool
	// Written is the number of bytes written to this client.
	Written int64
	// Discarded is the number of bytes discarded (e.g. when the client is slow).
	Discarded int64
	// Subscriptions holds named notification subscriptions registered via
	// refresh-client -B name:notify:format.
	Subscriptions map[string]Subscription
	// ClipboardData holds the most recent clipboard content received via an
	// OSC 52 response from the client terminal.
	ClipboardData string
}

// NewClient creates a Client with sensible defaults. The Session field is nil
// until [Server.AttachClient] is called.
func NewClient(id ClientID) *Client {
	return &Client{
		ID:        id,
		KeyTable:  "root",
		Env:       make(Environ),
		CreatedAt: time.Now(),
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
// Recognised keys: all client_* format variables.
func (c *Client) Lookup(key string) (string, bool) {
	switch key {
	case "client_id":
		return string(c.ID), true
	case "client_tty":
		return c.TTY, true
	case "client_term", "client_termname", "client_termtype":
		return c.Term, true
	case "client_width":
		return fmt.Sprintf("%d", c.Size.Cols), true
	case "client_height":
		return fmt.Sprintf("%d", c.Size.Rows), true
	case "client_key_table":
		return c.KeyTable, true
	case "client_cwd":
		return c.Cwd, true
	case "client_activity":
		return fmt.Sprintf("%d", c.CreatedAt.Unix()), true
	case "client_cell_height", "client_cell_width":
		return "0", true
	case "client_created":
		return fmt.Sprintf("%d", c.CreatedAt.Unix()), true
	case "client_discarded":
		return fmt.Sprintf("%d", c.Discarded), true
	case "client_flags":
		return "", true
	case "client_last_session":
		return c.LastSession, true
	case "client_name":
		return string(c.ID), true
	case "client_pid":
		return fmt.Sprintf("%d", c.PID), true
	case "client_prefix":
		return boolVal(c.Prefix), true
	case "client_readonly":
		return boolVal(c.Readonly), true
	case "client_session":
		if c.Session != nil {
			return c.Session.Name, true
		}
		return "", true
	case "client_termfeatures":
		return "", true
	case "client_uid":
		return fmt.Sprintf("%d", c.UID), true
	case "client_user":
		return c.UserName, true
	case "client_written":
		return fmt.Sprintf("%d", c.Written), true
	}
	return "", false
}

// Children satisfies [format.Context]. No list keys are defined for Client.
func (c *Client) Children(_ string) []format.Context {
	return nil
}
