package server

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"

	"github.com/dhamidi/dmux/internal/command"
	"github.com/dhamidi/dmux/internal/control"
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/layout"
	"github.com/dhamidi/dmux/internal/options"
	"github.com/dhamidi/dmux/internal/parse"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/session"
	"github.com/dhamidi/dmux/internal/shell"
)

// serverMutator wraps *session.Server and implements command.Mutator.
// Session, window, pane, and key-binding mutations are stubs pending
// their respective implementation layers; buffer mutations are fully
// wired to srv.Buffers.
type serverMutator struct {
	state         *session.Server
	store         command.Server
	queue         *command.Queue
	nextSessionID uint64
	nextWindowID  uint64
	shutdown      func()
	getConn       func(session.ClientID) (*clientConn, bool)
	markDirty     func(*clientConn)
	// clearPrevFrame, if non-nil, is called to invalidate the cached previous
	// frame for a client so the next render performs a full repaint.
	clearPrevFrame func(session.ClientID)
	// newPane creates a new pane from the given configuration.
	// Injected from Run (real PTY) or tests (fake).
	newPane func(cfg pane.Config) (session.Pane, error)
	// watchPane, if non-nil, is called after each new pane is created.
	// It starts a goroutine that fires pane-died when the process exits.
	watchPane func(paneID int)
	// events is the server-wide event bus for publishing state-change events
	// to control-mode clients. May be nil if no event bus is configured.
	events *control.EventBus
}

// newServerMutator returns a *serverMutator backed by state.
// shutdown is called by KillServer to trigger a graceful shutdown.
// getConn and markDirty provide access to the live connection map for
// operations that need to write directly to a client's network connection.
// newPaneFn is the factory used to create new panes; pass a fake in tests.
// The clearPrevFrame field may be set on the returned value to enable
// frame-cache invalidation on explicit full-refresh requests.
func newServerMutator(
	state *session.Server,
	store command.Server,
	queue *command.Queue,
	shutdown func(),
	getConn func(session.ClientID) (*clientConn, bool),
	markDirty func(*clientConn),
	newPaneFn func(cfg pane.Config) (session.Pane, error),
	watchPaneFn func(paneID int),
	eventBus ...*control.EventBus,
) *serverMutator {
	m := &serverMutator{
		state:     state,
		store:     store,
		queue:     queue,
		shutdown:  shutdown,
		getConn:   getConn,
		markDirty: markDirty,
		newPane:   newPaneFn,
		watchPane: watchPaneFn,
	}
	if len(eventBus) > 0 {
		m.events = eventBus[0]
	}
	return m
}

// notifyAll publishes e to the server event bus if one is configured.
func (m *serverMutator) notifyAll(e control.Event) {
	if m.events != nil {
		m.events.Publish(e)
	}
}

func errStub(method string) error {
	return fmt.Errorf("%s: not yet implemented", method)
}

func (m *serverMutator) NewSession(name string) (command.SessionView, error) {
	m.nextSessionID++
	id := session.SessionID(fmt.Sprintf("s%d", m.nextSessionID))
	if name == "" {
		name = fmt.Sprintf("session%d", m.nextSessionID)
	}
	sess := session.NewSession(id, name, m.state.Options)
	m.state.AddSession(sess)
	v := command.SessionView{
		ID:      string(id),
		Name:    name,
		Windows: []command.WindowView{},
		Current: -1,
	}
	m.notifyAll(control.SessionsChangedEvent{})
	m.RunHook("after-new-session")
	return v, nil
}

func (m *serverMutator) KillSession(id string) error {
	sess, ok := m.state.Sessions[session.SessionID(id)]
	if !ok {
		return fmt.Errorf("kill-session: session %q not found", id)
	}
	for _, wl := range sess.Windows {
		for paneID, pane := range wl.Window.Panes {
			if err := pane.Close(); err != nil {
				log.Printf("kill-session: closing pane %v: %v", paneID, err)
			}
		}
	}
	m.state.RemoveSession(session.SessionID(id))
	m.notifyAll(control.SessionsChangedEvent{})
	m.RunHook("after-kill-session")
	return nil
}

func (m *serverMutator) RenameSession(id, name string) error {
	sess, ok := m.state.Sessions[session.SessionID(id)]
	if !ok {
		return fmt.Errorf("rename-session: session %q not found", id)
	}
	sess.Name = name
	m.notifyAll(control.SessionRenamedEvent{SessionID: id, Name: name})
	return nil
}

func (m *serverMutator) AttachClient(clientID, sessionID string) error {
	c, ok := m.state.Clients[session.ClientID(clientID)]
	if !ok {
		return fmt.Errorf("attach-client: client %q not found", clientID)
	}
	if err := m.state.AttachClient(c, session.SessionID(sessionID)); err != nil {
		return err
	}
	if sess, ok := m.state.Sessions[session.SessionID(sessionID)]; ok {
		m.notifyAll(control.SessionChangedEvent{SessionID: sessionID, Name: sess.Name})
	}
	m.RunHook("client-attached")
	return nil
}

func (m *serverMutator) DetachClient(clientID string) error {
	if _, ok := m.state.Clients[session.ClientID(clientID)]; !ok {
		return fmt.Errorf("detach-client: client %q not found", clientID)
	}
	m.state.DetachClient(session.ClientID(clientID))
	return nil
}

func (m *serverMutator) SwitchClient(clientID, sessionID string) error {
	client, ok := m.state.Clients[session.ClientID(clientID)]
	if !ok {
		return fmt.Errorf("switch-client: client %q not found", clientID)
	}
	targetSession, ok := m.state.Sessions[session.SessionID(sessionID)]
	if !ok {
		return fmt.Errorf("switch-client: session %q not found", sessionID)
	}
	client.Session = targetSession
	return nil
}

func (m *serverMutator) NewWindow(sessionID, name string) (command.WindowView, error) {
	sess, ok := m.state.Sessions[session.SessionID(sessionID)]
	if !ok {
		return command.WindowView{}, fmt.Errorf("new-window: session %q not found", sessionID)
	}

	m.nextWindowID++
	id := session.WindowID(fmt.Sprintf("w%d", m.nextWindowID))
	if name == "" {
		name = fmt.Sprintf("window%d", m.nextWindowID)
	}

	win := session.NewWindow(id, name, sess.Options)

	// Determine client size: use the first attached client's size, defaulting to 80×24.
	cols, rows := 80, 24
	for _, c := range m.state.Clients {
		if c.Session != nil && c.Session.ID == sess.ID {
			if c.Size.Cols > 0 {
				cols = c.Size.Cols
			}
			if c.Size.Rows > 0 {
				rows = c.Size.Rows
			}
			break
		}
	}

	paneID := session.PaneID(layout.LeafID(1))
	p, err := m.newPane(pane.Config{ID: paneID})
	if err != nil {
		return command.WindowView{}, fmt.Errorf("new-window: creating pane: %w", err)
	}

	win.AddPane(paneID, p)
	win.Layout = layout.New(cols, rows, paneID)

	wl := sess.AddWindow(win)
	if m.watchPane != nil {
		m.watchPane(int(paneID))
	}
	m.notifyAll(control.WindowAddEvent{WindowID: string(id)})
	m.RunHook("after-new-window")
	return toWindowView(wl), nil
}

func (m *serverMutator) KillWindow(sessionID, windowID string) error {
	sess, ok := m.state.Sessions[session.SessionID(sessionID)]
	if !ok {
		return fmt.Errorf("kill-window: session %q not found", sessionID)
	}

	for i, wl := range sess.Windows {
		if wl.Window.ID == session.WindowID(windowID) {
			win := wl.Window
			for paneID, p := range win.Panes {
				if err := p.Close(); err != nil {
					log.Printf("kill-window: closing pane %v: %v", paneID, err)
				}
			}
			sess.RemoveWindow(i)
			m.notifyAll(control.WindowCloseEvent{WindowID: windowID})
			m.RunHook("after-kill-window")
			return nil
		}
	}
	return fmt.Errorf("kill-window: window %q not found in session %q", windowID, sessionID)
}

func (m *serverMutator) RenameWindow(sessionID, windowID, name string) error {
	sess, ok := m.state.Sessions[session.SessionID(sessionID)]
	if !ok {
		return fmt.Errorf("rename-window: session %q not found", sessionID)
	}
	for _, wl := range sess.Windows {
		if wl.Window.ID == session.WindowID(windowID) {
			wl.Window.Name = name
			m.notifyAll(control.WindowRenamedEvent{WindowID: windowID, Name: name})
			return nil
		}
	}
	return fmt.Errorf("rename-window: window %q not found in session %q", windowID, sessionID)
}

func (m *serverMutator) SelectWindow(sessionID, windowID string) error {
	sess, ok := m.state.Sessions[session.SessionID(sessionID)]
	if !ok {
		return fmt.Errorf("select-window: session %q not found", sessionID)
	}
	for _, wl := range sess.Windows {
		if wl.Window.ID == session.WindowID(windowID) {
			if sess.Current != nil && sess.Current.Window.ID != session.WindowID(windowID) {
				sess.LastWindowID = sess.Current.Window.ID
			}
			sess.Current = wl
			wl.Window.ActivityFlag = false
			m.RunHook("after-select-window")
			return nil
		}
	}
	return fmt.Errorf("select-window: window %q not found in session %q", windowID, sessionID)
}

func (m *serverMutator) SplitWindow(sessionID, windowID string) (command.PaneView, error) {
	sess, ok := m.state.Sessions[session.SessionID(sessionID)]
	if !ok {
		return command.PaneView{}, fmt.Errorf("split-window: session %q not found", sessionID)
	}

	var win *session.Window
	for _, wl := range sess.Windows {
		if wl.Window.ID == session.WindowID(windowID) {
			win = wl.Window
			break
		}
	}
	if win == nil {
		return command.PaneView{}, fmt.Errorf("split-window: window %q not found in session %q", windowID, sessionID)
	}

	activePaneID := win.Active
	newPaneID := win.Layout.Split(activePaneID, layout.Vertical)

	p, err := m.newPane(pane.Config{ID: newPaneID})
	if err != nil {
		return command.PaneView{}, fmt.Errorf("split-window: creating pane: %w", err)
	}

	win.AddPane(newPaneID, p)
	if m.watchPane != nil {
		m.watchPane(int(newPaneID))
	}
	m.RunHook("after-split-window")
	return command.PaneView{
		ID:    int(newPaneID),
		Title: p.Title(),
	}, nil
}

func (m *serverMutator) KillPane(paneID int) error {
	targetID := session.PaneID(paneID)

	for _, sess := range m.state.Sessions {
		for _, wl := range sess.Windows {
			win := wl.Window
			p, ok := win.Panes[targetID]
			if !ok {
				continue
			}
			if err := p.Close(); err != nil {
				log.Printf("kill-pane: closing pane %v: %v", targetID, err)
			}
			win.RemovePane(targetID)
			win.Layout.Close(targetID)
			if len(win.Panes) == 0 {
				return m.KillWindow(string(sess.ID), string(win.ID))
			}
			m.RunHook("after-kill-pane")
			return nil
		}
	}
	return fmt.Errorf("kill-pane: pane %d not found", paneID)
}

func (m *serverMutator) SelectPane(sessionID, windowID string, paneID int) error {
	sess, ok := m.state.Sessions[session.SessionID(sessionID)]
	if !ok {
		return fmt.Errorf("select-pane: session %q not found", sessionID)
	}
	var win *session.Window
	for _, wl := range sess.Windows {
		if wl.Window.ID == session.WindowID(windowID) {
			win = wl.Window
			break
		}
	}
	if win == nil {
		return fmt.Errorf("select-pane: window %q not found in session %q", windowID, sessionID)
	}
	targetID := session.PaneID(paneID)
	if _, ok := win.Panes[targetID]; !ok {
		return fmt.Errorf("select-pane: pane %d not found in window %q", paneID, windowID)
	}
	if win.Active != 0 && win.Active != targetID {
		win.LastPaneID = win.Active
	}
	win.Active = targetID
	return nil
}

func (m *serverMutator) ResizePane(paneID int, direction string, amount int) error {
	sess, win, _, err := m.findPane(paneID)
	if err != nil {
		return fmt.Errorf("resize-pane: %w", err)
	}
	if win.Layout == nil {
		return fmt.Errorf("resize-pane: window has no layout")
	}

	targetID := session.PaneID(paneID)

	// Z toggles zoom; no border movement needed.
	if direction == "Z" {
		// Zoom is not yet implemented in the layout package; treat as no-op.
		return nil
	}

	// Map direction string to the layout edge to move.
	var edge layout.Edge
	switch direction {
	case "U":
		edge = layout.EdgeTop
	case "D":
		edge = layout.EdgeBottom
	case "L":
		edge = layout.EdgeLeft
	case "R":
		edge = layout.EdgeRight
	default:
		return fmt.Errorf("resize-pane: unknown direction %q", direction)
	}

	win.Layout.MoveBorder(targetID, edge, amount)

	// Resize every pane's PTY to match the updated layout rectangle.
	for id, p := range win.Panes {
		r := win.Layout.Rect(id)
		if r.Width > 0 && r.Height > 0 {
			_ = p.Resize(r.Width, r.Height) // best-effort
		}
	}

	// Trigger a redraw for all clients attached to the session.
	m.markSessionDirty(sess)
	return nil
}

// markSessionDirty marks all clients attached to sess as needing a redraw.
func (m *serverMutator) markSessionDirty(sess *session.Session) {
	if m.markDirty == nil || m.getConn == nil {
		return
	}
	for _, c := range m.state.Clients {
		if c.Session == sess {
			if conn, ok := m.getConn(c.ID); ok {
				m.markDirty(conn)
			}
		}
	}
}

func (m *serverMutator) CapturePane(paneID int, history bool) (string, error) {
	return "", errStub("capture-pane")
}

func (m *serverMutator) RespawnPane(paneID int, shell string) error {
	return errStub("respawn-pane")
}

func (m *serverMutator) BindKey(table, keyStr, cmd string) error {
	k, err := keys.Parse(keyStr)
	if err != nil {
		return fmt.Errorf("bind-key: %w", err)
	}
	t, ok := m.state.KeyTables.Get(table)
	if !ok {
		t = keys.NewTable()
		m.state.KeyTables.Register(table, t)
	}
	t.Bind(k, cmd)
	return nil
}

func (m *serverMutator) UnbindKey(table, key string) error {
	t, ok := m.state.KeyTables.Get(table)
	if !ok {
		return fmt.Errorf("unbind-key: table %q not found", table)
	}
	k, err := keys.Parse(key)
	if err != nil {
		return fmt.Errorf("unbind-key: %w", err)
	}
	t.Unbind(k)
	return nil
}

func (m *serverMutator) ListKeyBindings(table string) []command.KeyBinding {
	var out []command.KeyBinding
	collect := func(name string, t *keys.Table) {
		t.Each(func(k keys.Key, cmd keys.BoundCommand) {
			out = append(out, command.KeyBinding{
				Table:   name,
				Key:     k.String(),
				Command: fmt.Sprintf("%v", cmd),
			})
		})
	}
	if table != "" {
		if t, ok := m.state.KeyTables.Get(table); ok {
			collect(table, t)
		}
		return out
	}
	for _, name := range m.state.KeyTables.Names() {
		t, _ := m.state.KeyTables.Get(name)
		collect(name, t)
	}
	return out
}

// resolveOptionStore maps a scope string to the appropriate *options.Store.
// Scope formats: "server", "session:<id>", "window:<sessionID>:<windowID>".
func (m *serverMutator) resolveOptionStore(scope string) (*options.Store, error) {
	parts := strings.SplitN(scope, ":", 3)
	switch parts[0] {
	case "server":
		return m.state.Options, nil
	case "session":
		if len(parts) < 2 {
			return nil, fmt.Errorf("session scope requires session ID")
		}
		sess, ok := m.state.Sessions[session.SessionID(parts[1])]
		if !ok {
			return nil, fmt.Errorf("session %q not found", parts[1])
		}
		return sess.Options, nil
	case "window":
		if len(parts) < 3 {
			return nil, fmt.Errorf("window scope requires session ID and window ID")
		}
		sess, ok := m.state.Sessions[session.SessionID(parts[1])]
		if !ok {
			return nil, fmt.Errorf("session %q not found", parts[1])
		}
		for _, wl := range sess.Windows {
			if wl.Window.ID == session.WindowID(parts[2]) {
				return wl.Window.Options, nil
			}
		}
		return nil, fmt.Errorf("window %q not found in session %q", parts[2], parts[1])
	default:
		return nil, fmt.Errorf("unrecognised scope %q", scope)
	}
}

func (m *serverMutator) SetOption(scope, name, value string) error {
	store, err := m.resolveOptionStore(scope)
	if err != nil {
		return fmt.Errorf("set-option: %w", err)
	}
	if _, ok := store.Get(name); !ok {
		store.Register(name, options.String, "")
	}
	return store.Set(name, value)
}

func (m *serverMutator) UnsetOption(scope, name string) error {
	store, err := m.resolveOptionStore(scope)
	if err != nil {
		return fmt.Errorf("unset-option: %w", err)
	}
	store.Unset(name)
	return nil
}

func (m *serverMutator) ListOptions(scope string) []command.OptionEntry {
	store, err := m.resolveOptionStore(scope)
	if err != nil {
		return nil
	}
	var out []command.OptionEntry
	store.Each(func(name string, value options.Value) {
		out = append(out, command.OptionEntry{Name: name, Value: value.String()})
	})
	return out
}

func (m *serverMutator) KillServer() error {
	m.shutdown()
	return nil
}

func (m *serverMutator) DisplayMessage(clientID, msg string) error {
	cc, ok := m.getConn(session.ClientID(clientID))
	if !ok {
		return fmt.Errorf("display-message: client %q not found", clientID)
	}
	encoded := proto.StdoutMsg{Data: []byte(msg + "\r\n")}.Encode()
	if err := proto.WriteMsg(cc.netConn, proto.MsgStdout, encoded); err != nil {
		return fmt.Errorf("display-message: %w", err)
	}
	m.markDirty(cc)
	return nil
}

// findPane scans all sessions and windows for the pane with the given ID.
func (m *serverMutator) findPane(paneID int) (sess *session.Session, win *session.Window, p session.Pane, err error) {
	targetID := session.PaneID(paneID)
	for _, s := range m.state.Sessions {
		for _, wl := range s.Windows {
			w := wl.Window
			if pane, ok := w.Panes[targetID]; ok {
				return s, w, pane, nil
			}
		}
	}
	return nil, nil, nil, fmt.Errorf("pane %d not found", paneID)
}

// findWindow scans all sessions for the window with the given IDs.
func (m *serverMutator) findWindow(sessionID, windowID string) (*session.Window, error) {
	sess, ok := m.state.Sessions[session.SessionID(sessionID)]
	if !ok {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	for _, wl := range sess.Windows {
		if wl.Window.ID == session.WindowID(windowID) {
			return wl.Window, nil
		}
	}
	return nil, fmt.Errorf("window %q not found in session %q", windowID, sessionID)
}

// presetCycle is the ordered list of layout presets used by select-layout -n/-p.
var presetCycle = []string{
	"even-horizontal",
	"even-vertical",
	"main-horizontal",
	"main-vertical",
	"tiled",
}

func (m *serverMutator) ApplyLayout(sessionID, windowID, layoutSpec string) error {
	win, err := m.findWindow(sessionID, windowID)
	if err != nil {
		return fmt.Errorf("select-layout: %w", err)
	}
	if win.Layout == nil {
		return fmt.Errorf("select-layout: window %q has no layout", windowID)
	}

	// Save current layout for undo.
	prev := win.Layout.Marshal()

	switch layoutSpec {
	case "undo":
		if win.LastLayout == "" {
			return fmt.Errorf("select-layout: no previous layout to restore")
		}
		t, err := layout.Unmarshal(win.LastLayout)
		if err != nil {
			return fmt.Errorf("select-layout: %w", err)
		}
		win.LastLayout = prev
		win.Layout = t
		return nil
	case "next", "prev":
		cur := win.CurrentPreset
		idx := -1
		for i, p := range presetCycle {
			if p == cur {
				idx = i
				break
			}
		}
		n := len(presetCycle)
		var next int
		if layoutSpec == "next" {
			next = (idx + 1 + n) % n
		} else {
			next = (idx - 1 + n) % n
		}
		layoutSpec = presetCycle[next]
		// fall through to apply the resolved preset name
		fallthrough
	case "even-horizontal", "even-vertical", "main-horizontal", "main-vertical", "tiled", "even":
		win.LastLayout = prev
		switch layoutSpec {
		case "even-horizontal":
			win.Layout.ApplyPreset(layout.PresetEvenHorizontal)
			win.CurrentPreset = "even-horizontal"
		case "even-vertical":
			win.Layout.ApplyPreset(layout.PresetEvenVertical)
			win.CurrentPreset = "even-vertical"
		case "main-horizontal":
			mainHeight := 0
			if opt, ok := win.Options.Get("main-pane-height"); ok && opt.Kind == options.Int {
				mainHeight = opt.Integer
			}
			win.Layout.ApplyPresetSized(layout.PresetMainHorizontal, mainHeight)
			win.CurrentPreset = "main-horizontal"
		case "main-vertical":
			mainWidth := 0
			if opt, ok := win.Options.Get("main-pane-width"); ok && opt.Kind == options.Int {
				mainWidth = opt.Integer
			}
			win.Layout.ApplyPresetSized(layout.PresetMainVertical, mainWidth)
			win.CurrentPreset = "main-vertical"
		case "tiled":
			win.Layout.ApplyPreset(layout.PresetTiled)
			win.CurrentPreset = "tiled"
		case "even":
			if win.Layout.Cols() >= win.Layout.Rows() {
				win.Layout.ApplyPreset(layout.PresetEvenHorizontal)
				win.CurrentPreset = "even-horizontal"
			} else {
				win.Layout.ApplyPreset(layout.PresetEvenVertical)
				win.CurrentPreset = "even-vertical"
			}
		}
		return nil
	default:
		// Try to parse as a serialised layout string.
		t, err := layout.Unmarshal(layoutSpec)
		if err != nil {
			return fmt.Errorf("select-layout: unknown layout %q: %w", layoutSpec, err)
		}
		win.LastLayout = prev
		win.Layout = t
		win.CurrentPreset = ""
		return nil
	}
}

func (m *serverMutator) RotateWindow(sessionID, windowID string, forward bool) error {
	win, err := m.findWindow(sessionID, windowID)
	if err != nil {
		return fmt.Errorf("rotate-window: %w", err)
	}
	if win.Layout == nil {
		return fmt.Errorf("rotate-window: window %q has no layout", windowID)
	}
	win.Layout.RotateLeaves(forward)
	return nil
}

func (m *serverMutator) ResizeWindow(sessionID, windowID string, cols, rows int) error {
	win, err := m.findWindow(sessionID, windowID)
	if err != nil {
		return fmt.Errorf("resize-window: %w", err)
	}
	if win.Layout == nil {
		return fmt.Errorf("resize-window: window %q has no layout", windowID)
	}
	win.Layout.Resize(cols, rows)
	// Resize each pane's PTY to its new bounds.
	for id, p := range win.Panes {
		r := win.Layout.Rect(id)
		if r.Width > 0 && r.Height > 0 {
			_ = p.Resize(r.Width, r.Height) // best-effort
		}
	}
	return nil
}

func (m *serverMutator) SendKeys(paneID int, keyStrs []string) error {
	_, _, p, err := m.findPane(paneID)
	if err != nil {
		return fmt.Errorf("send-keys: %w", err)
	}
	for _, keyStr := range keyStrs {
		k, err := keys.Parse(keyStr)
		if err != nil {
			return fmt.Errorf("send-keys: %w", err)
		}
		if err := p.SendKey(k); err != nil {
			return fmt.Errorf("send-keys: %w", err)
		}
	}
	return nil
}

func (m *serverMutator) RunShell(cmd string, background bool) (string, error) {
	shellPath := shell.Default(os.LookupEnv, func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	})
	if background {
		if err := exec.Command(shellPath, "-c", cmd).Start(); err != nil {
			return "", fmt.Errorf("run-shell: %w", err)
		}
		return "", nil
	}
	out, err := exec.Command(shellPath, "-c", cmd).CombinedOutput()
	return string(out), err
}

// ─── Buffer mutations ────────────────────────────────────────────────────────

func (m *serverMutator) SetBuffer(name, data string) error {
	m.state.Buffers.Set(name, data)
	return nil
}

func (m *serverMutator) DeleteBuffer(name string) error {
	if !m.state.Buffers.DeleteNamed(name) {
		return fmt.Errorf("delete-buffer: buffer %q not found", name)
	}
	return nil
}

func (m *serverMutator) LoadBuffer(name, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("load-buffer: %w", err)
	}
	m.state.Buffers.Set(name, string(data))
	return nil
}

func (m *serverMutator) SaveBuffer(name, path string) error {
	buf, ok := m.state.Buffers.GetNamed(name)
	if !ok {
		return fmt.Errorf("save-buffer: buffer %q not found", name)
	}
	if err := os.WriteFile(path, buf.Data, 0o644); err != nil {
		return fmt.Errorf("save-buffer: %w", err)
	}
	return nil
}

func (m *serverMutator) PasteBuffer(name string, paneID int) error {
	buf, ok := m.state.Buffers.GetNamed(name)
	if !ok {
		return fmt.Errorf("paste-buffer: buffer %q not found", name)
	}
	_, _, p, err := m.findPane(paneID)
	if err != nil {
		return fmt.Errorf("paste-buffer: %w", err)
	}
	if err := p.Write(buf.Data); err != nil {
		return fmt.Errorf("paste-buffer: %w", err)
	}
	return nil
}

func (m *serverMutator) ListBuffers() []command.BufferEntry {
	bufs := m.state.Buffers.List()
	out := make([]command.BufferEntry, len(bufs))
	for i, b := range bufs {
		out[i] = command.BufferEntry{Name: b.Name, Size: len(b.Data)}
	}
	return out
}

// ─── Window movement ─────────────────────────────────────────────────────────

func (m *serverMutator) LinkWindow(srcSessionID, srcWindowID, dstSessionID string, index int, afterIndex, beforeIndex, selectWin, killExisting bool) error {
	srcSess, ok := m.state.Sessions[session.SessionID(srcSessionID)]
	if !ok {
		return fmt.Errorf("link-window: source session %q not found", srcSessionID)
	}

	var win *session.Window
	for _, wl := range srcSess.Windows {
		if wl.Window.ID == session.WindowID(srcWindowID) {
			win = wl.Window
			break
		}
	}
	if win == nil {
		return fmt.Errorf("link-window: window %q not found in session %q", srcWindowID, srcSessionID)
	}

	dstSess, ok := m.state.Sessions[session.SessionID(dstSessionID)]
	if !ok {
		return fmt.Errorf("link-window: destination session %q not found", dstSessionID)
	}

	// Compute the display index for the new winlink.
	var insertIdx int
	if index < 0 {
		// Append after the last existing index.
		if len(dstSess.Windows) > 0 {
			insertIdx = dstSess.Windows[len(dstSess.Windows)-1].Index + 1
		} else {
			insertIdx = 1
		}
	} else if afterIndex {
		insertIdx = index + 1
	} else {
		insertIdx = index
	}

	// Kill any existing window at insertIdx if -k was given.
	if killExisting {
		for i, wl := range dstSess.Windows {
			if wl.Index == insertIdx {
				w := wl.Window
				for paneID, p := range w.Panes {
					if err := p.Close(); err != nil {
						log.Printf("link-window: closing pane %v: %v", paneID, err)
					}
				}
				dstSess.RemoveWindow(i)
				break
			}
		}
	}

	// If a window still occupies the target index, shift all windows at
	// >= insertIdx up by one to make room.
	for _, wl := range dstSess.Windows {
		if wl.Index >= insertIdx {
			wl.Index++
		}
	}

	// Build and insert the new winlink in sorted order.
	newWL := &session.Winlink{Index: insertIdx, Window: win}
	pos := len(dstSess.Windows)
	for i, wl := range dstSess.Windows {
		if wl.Index > insertIdx {
			pos = i
			break
		}
	}
	dstSess.Windows = append(dstSess.Windows[:pos], append([]*session.Winlink{newWL}, dstSess.Windows[pos:]...)...)

	if dstSess.Current == nil {
		dstSess.Current = newWL
	} else if selectWin {
		dstSess.Current = newWL
	}

	// Record the link in the window itself.
	win.AddLinkedSession(session.SessionID(srcSessionID))
	win.AddLinkedSession(session.SessionID(dstSessionID))

	m.RunHook("after-link-window")
	return nil
}

func (m *serverMutator) UnlinkWindow(sessionID, windowID string, kill bool) error {
	sess, ok := m.state.Sessions[session.SessionID(sessionID)]
	if !ok {
		return fmt.Errorf("unlink-window: session %q not found", sessionID)
	}

	var win *session.Window
	for i, wl := range sess.Windows {
		if wl.Window.ID == session.WindowID(windowID) {
			win = wl.Window
			sess.RemoveWindow(i)
			break
		}
	}
	if win == nil {
		return fmt.Errorf("unlink-window: window %q not found in session %q", windowID, sessionID)
	}

	win.RemoveLinkedSession(session.SessionID(sessionID))

	if kill && len(win.LinkedSessions) == 0 {
		for paneID, p := range win.Panes {
			if err := p.Close(); err != nil {
				log.Printf("unlink-window: closing pane %v: %v", paneID, err)
			}
		}
		win.Panes = make(map[session.PaneID]session.Pane)
	}

	m.RunHook("after-unlink-window")
	return nil
}

func (m *serverMutator) MoveWindow(sessionID, windowID string, newIndex int) error {
	return errStub("move-window")
}

func (m *serverMutator) SwapWindows(sessionID, aWindowID, bWindowID string) error {
	return errStub("swap-window")
}

func (m *serverMutator) FindWindow(sessionID, pattern string) (command.WindowView, error) {
	return command.WindowView{}, errStub("find-window")
}

// ─── Pane movement ───────────────────────────────────────────────────────────

func (m *serverMutator) SwapPane(sessionID, windowID string, paneA, paneB int) error {
	return errStub("swap-pane")
}

func (m *serverMutator) BreakPane(sessionID, windowID string, paneID int) (command.WindowView, error) {
	return command.WindowView{}, errStub("break-pane")
}

func (m *serverMutator) JoinPane(srcSessionID, srcWindowID string, srcPaneID int, dstSessionID, dstWindowID string) error {
	return errStub("join-pane")
}

func (m *serverMutator) MovePane(srcSessionID, srcWindowID string, srcPaneID int, dstSessionID, dstWindowID string) error {
	return errStub("move-pane")
}

func (m *serverMutator) SlicePane(sessionID, windowID string, paneID int) (command.PaneView, error) {
	return command.PaneView{}, errStub("slice-pane")
}

func (m *serverMutator) RespawnWindow(sessionID, windowID, shell, dir string) error {
	return errStub("respawn-window")
}

// ─── Pane pipe / clear operations ────────────────────────────────────────────

func (m *serverMutator) PipePane(paneID int, shellCmd string, inFlag, outFlag, onceFlag bool) error {
	return errStub("pipe-pane")
}

func (m *serverMutator) ClearHistory(paneID int, visibleToo bool) error {
	_, _, p, err := m.findPane(paneID)
	if err != nil {
		return fmt.Errorf("clear-history: %w", err)
	}
	p.ClearHistory()
	if visibleToo {
		if err := p.ClearScreen(); err != nil {
			return fmt.Errorf("clear-history: clear screen: %w", err)
		}
	}
	return nil
}

func (m *serverMutator) ClearPane(paneID int) error {
	_, _, p, err := m.findPane(paneID)
	if err != nil {
		return fmt.Errorf("clear-pane: %w", err)
	}
	return p.ClearScreen()
}

// ─── Environment mutations ────────────────────────────────────────────────────

// resolveEnviron maps a scope string to the appropriate session.Environ.
// Scope "global" or "server" maps to the server's global Env. Any other
// string is interpreted as a session ID.
func (m *serverMutator) resolveEnviron(scope string) (session.Environ, error) {
	switch scope {
	case "global", "server":
		return m.state.Env, nil
	default:
		sess, ok := m.state.Sessions[session.SessionID(scope)]
		if !ok {
			return nil, fmt.Errorf("session %q not found", scope)
		}
		return sess.Env, nil
	}
}

func (m *serverMutator) SetEnvironment(scope, name, value string, remove bool) error {
	env, err := m.resolveEnviron(scope)
	if err != nil {
		return fmt.Errorf("set-environment: %w", err)
	}
	if remove {
		env.Remove(name)
	} else {
		env.Set(name, value)
	}
	return nil
}

func (m *serverMutator) ListEnvironment(scope string) []command.EnvironEntry {
	env, err := m.resolveEnviron(scope)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(env))
	for k := range env {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]command.EnvironEntry, len(names))
	for i, k := range names {
		out[i] = command.EnvironEntry{Name: k, Value: env[k]}
	}
	return out
}

// ─── Server management ───────────────────────────────────────────────────────

func (m *serverMutator) ShowMessages() []string {
	return m.state.Messages
}

func (m *serverMutator) LockServer() error {
	// No real screen-lock mechanism in this implementation.
	return nil
}

func (m *serverMutator) LockClient(clientID string) error {
	// No real screen-lock mechanism in this implementation.
	return nil
}

func (m *serverMutator) WaitFor(channel string) error {
	return m.state.Channels.Wait(channel)
}

func (m *serverMutator) SignalChannel(channel string) {
	m.state.Channels.Signal(channel)
}

// ─── Mode entry mutations ─────────────────────────────────────────────────────

func (m *serverMutator) EnterCopyMode(clientID string, scrollback bool) error {
	return errStub("copy-mode")
}

func (m *serverMutator) EnterChooseTree(clientID, sessionID, windowID string) error {
	return errStub("choose-tree")
}

func (m *serverMutator) EnterCustomizeMode(clientID string) error {
	return errStub("customize-mode")
}

func (m *serverMutator) EnterChooseBuffer(clientID, windowID string, items []command.ChooserItem, template string) error {
	return errStub("choose-buffer")
}

func (m *serverMutator) EnterChooseClient(clientID, windowID string, items []command.ChooserItem, template string) error {
	return errStub("choose-client")
}

func (m *serverMutator) EnterClockMode(clientID string, paneID int) error {
	return errStub("clock-mode")
}

func (m *serverMutator) DisplayPopup(clientID, cmd, title string, cols, rows int) error {
	return errStub("display-popup")
}

func (m *serverMutator) DisplayMenu(clientID string, items []command.MenuEntry) error {
	return errStub("display-menu")
}

func (m *serverMutator) DisplayPanes(clientID string) error {
	return errStub("display-panes")
}

func (m *serverMutator) CommandPrompt(clientID, prompt, initialValue string) error {
	return errStub("command-prompt")
}

func (m *serverMutator) ConfirmBefore(clientID, prompt, cmd string) error {
	return errStub("confirm-before")
}

// ─── Hook mutations ───────────────────────────────────────────────────────────

// SetHook registers a compiled command callback for event.
// Passing cmd="" removes all hooks for event.
func (m *serverMutator) SetHook(event, cmd string) error {
	if cmd == "" {
		m.state.Hooks.Delete(event)
		return nil
	}
	cmds, err := parse.Parse(cmd)
	if err != nil {
		return fmt.Errorf("set-hook: parse %q: %w", cmd, err)
	}
	if len(cmds) == 0 {
		return fmt.Errorf("set-hook: empty command %q", cmd)
	}
	c := cmds[0]
	store := m.store
	queue := m.queue
	mut := command.Mutator(m)
	var nilClientView command.ClientView
	fn := func() {
		command.Dispatch(c.Name, c.Args, store, nilClientView, queue, mut)
	}
	// Replace existing hooks for this event with the new one.
	m.state.Hooks.Delete(event)
	m.state.Hooks.Register(event, cmd, fn)
	return nil
}

// RunHook fires all registered hooks for event synchronously.
func (m *serverMutator) RunHook(event string) {
	m.state.Hooks.Run(event)
}

// ListHooks returns all registered hooks as OptionEntry pairs where Name is
// the event name and Value is the command string.
func (m *serverMutator) ListHooks() []command.OptionEntry {
	entries := m.state.Hooks.List()
	out := make([]command.OptionEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, command.OptionEntry{Name: e.Event, Value: e.Cmd})
	}
	return out
}

// ─── Client display mutations ─────────────────────────────────────────────────

// RefreshClient triggers a full redraw for the named client. In this
// implementation the client is marked dirty so the render loop repaints it.
// The cached previous frame is invalidated so the next render sends a full
// repaint rather than an incremental diff.
func (m *serverMutator) RefreshClient(clientID string) error {
	c, ok := m.state.Clients[session.ClientID(clientID)]
	if !ok {
		return fmt.Errorf("refresh-client: client %q not found", clientID)
	}
	if m.clearPrevFrame != nil {
		m.clearPrevFrame(c.ID)
	}
	if conn, ok := m.getConn(c.ID); ok {
		m.markDirty(conn)
	}
	return nil
}

// ResizeClient updates the terminal dimensions of the named client.
func (m *serverMutator) ResizeClient(clientID string, cols, rows int) error {
	c, ok := m.state.Clients[session.ClientID(clientID)]
	if !ok {
		return fmt.Errorf("resize-client: client %q not found", clientID)
	}
	c.Size.Cols = cols
	c.Size.Rows = rows
	if conn, ok := m.getConn(c.ID); ok {
		m.markDirty(conn)
	}
	return nil
}

// SuspendClient sends SIGTSTP to the client process, returning it to the shell
// that launched dmux. On resume (SIGCONT) the client should redraw.
func (m *serverMutator) SuspendClient(clientID string) error {
	c, ok := m.state.Clients[session.ClientID(clientID)]
	if !ok {
		return fmt.Errorf("suspend-client: client %q not found", clientID)
	}
	if c.PID == 0 {
		return fmt.Errorf("suspend-client: client %q has no PID", clientID)
	}
	proc, err := os.FindProcess(c.PID)
	if err != nil {
		return fmt.Errorf("suspend-client: %v", err)
	}
	return proc.Signal(syscall.SIGTSTP)
}

// ─── Server access control ────────────────────────────────────────────────────

// SetServerAccess records an ACL entry for username on the server.
// allow=true grants access; allow=false denies it. write=true additionally
// grants write access.
//
// TODO: enforce this ACL in the connection-accept loop (server.Run) by
// looking up the connecting user's Unix username in m.state.ACL before
// accepting the connection.
func (m *serverMutator) SetServerAccess(username string, allow, write bool) error {
	if username == "" {
		return fmt.Errorf("server-access: username must not be empty")
	}
	m.state.ACL[username] = allow
	if write {
		m.state.ACLWriteAccess[username] = true
	} else {
		delete(m.state.ACLWriteAccess, username)
	}
	return nil
}

// DenyAllClients sets a flag that causes the server to reject all new
// incoming connections.
//
// TODO: enforce m.state.ACLDenyAll in the connection-accept loop (server.Run).
func (m *serverMutator) DenyAllClients() error {
	m.state.ACLDenyAll = true
	return nil
}
