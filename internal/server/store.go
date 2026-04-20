package server

import (
	"github.com/dhamidi/dmux/internal/command"
	"github.com/dhamidi/dmux/internal/session"
)

// serverStore implements [command.Server] as a thin read-only wrapper around
// a [*session.Server]. All returned view types are plain-data copies with no
// live pointers into the session graph.
type serverStore struct{ state *session.Server }

func newServerStore(s *session.Server) *serverStore { return &serverStore{s} }

// GetSession looks up a session by its string ID.
func (ss *serverStore) GetSession(id string) (command.SessionView, bool) {
	s, ok := ss.state.Sessions[session.SessionID(id)]
	if !ok {
		return command.SessionView{}, false
	}
	return toSessionView(s), true
}

// GetSessionByName returns the first session whose Name matches.
func (ss *serverStore) GetSessionByName(name string) (command.SessionView, bool) {
	for _, s := range ss.state.Sessions {
		if s.Name == name {
			return toSessionView(s), true
		}
	}
	return command.SessionView{}, false
}

// ListSessions returns a SessionView for every session in undefined order.
func (ss *serverStore) ListSessions() []command.SessionView {
	out := make([]command.SessionView, 0, len(ss.state.Sessions))
	for _, s := range ss.state.Sessions {
		out = append(out, toSessionView(s))
	}
	return out
}

// GetClient looks up a client by its string ID.
func (ss *serverStore) GetClient(id string) (command.ClientView, bool) {
	c, ok := ss.state.Clients[session.ClientID(id)]
	if !ok {
		return command.ClientView{}, false
	}
	return toClientView(c), true
}

// ListClients returns a ClientView for every connected client in undefined order.
func (ss *serverStore) ListClients() []command.ClientView {
	out := make([]command.ClientView, 0, len(ss.state.Clients))
	for _, c := range ss.state.Clients {
		out = append(out, toClientView(c))
	}
	return out
}

// toSessionView converts a *session.Session into a plain-data SessionView.
func toSessionView(s *session.Session) command.SessionView {
	windows := make([]command.WindowView, len(s.Windows))
	current := -1
	for i, wl := range s.Windows {
		windows[i] = toWindowView(wl)
		if wl == s.Current {
			current = i
		}
	}
	return command.SessionView{
		ID:           string(s.ID),
		Name:         s.Name,
		Windows:      windows,
		Current:      current,
		LastWindowID: string(s.LastWindowID),
	}
}

// toWindowView converts a *session.Winlink into a plain-data WindowView.
func toWindowView(wl *session.Winlink) command.WindowView {
	w := wl.Window
	panes := make([]command.PaneView, 0, len(w.Panes))
	for paneID, pane := range w.Panes {
		panes = append(panes, command.PaneView{
			ID:    int(paneID),
			Title: pane.Title(),
		})
	}
	cols, rows := 0, 0
	if w.Layout != nil {
		cols = w.Layout.Cols()
		rows = w.Layout.Rows()
	}
	linkedSessions := make([]string, len(w.LinkedSessions))
	for i, id := range w.LinkedSessions {
		linkedSessions[i] = string(id)
	}
	return command.WindowView{
		ID:             string(w.ID),
		Name:           w.Name,
		Index:          wl.Index,
		Panes:          panes,
		Active:         int(w.Active),
		LastPaneID:     int(w.LastPaneID),
		ActivityFlag:   w.ActivityFlag,
		Cols:           cols,
		Rows:           rows,
		LastLayout:     w.LastLayout,
		CurrentPreset:  w.CurrentPreset,
		LinkedSessions: linkedSessions,
	}
}

// toClientView converts a *session.Client into a plain-data ClientView.
func toClientView(c *session.Client) command.ClientView {
	sessionID := ""
	if c.Session != nil {
		sessionID = string(c.Session.ID)
	}
	return command.ClientView{
		ID:        string(c.ID),
		SessionID: sessionID,
		Cols:      c.Size.Cols,
		Rows:      c.Size.Rows,
		TTY:       c.TTY,
		KeyTable:  c.KeyTable,
	}
}
