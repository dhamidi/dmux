package server

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dhamidi/dmux/internal/command"
	"github.com/dhamidi/dmux/internal/control"
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/layout"
	"github.com/dhamidi/dmux/internal/modes"
	clockmode "github.com/dhamidi/dmux/internal/modes/clock"
	copymode "github.com/dhamidi/dmux/internal/modes/copy"
	displaypanes "github.com/dhamidi/dmux/internal/modes/displaypanes"
	menumode "github.com/dhamidi/dmux/internal/modes/menu"
	popupmode "github.com/dhamidi/dmux/internal/modes/popup"
	promptmode "github.com/dhamidi/dmux/internal/modes/prompt"
	treemode "github.com/dhamidi/dmux/internal/modes/tree"
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

	// pushOverlayFn, if non-nil, pushes ov onto the named client's overlay stack.
	// Wired to srv.pushOverlay by Run.
	pushOverlayFn func(id session.ClientID, ov modes.ClientOverlay)

	// popOverlayFn, if non-nil, removes the topmost overlay from the named
	// client's overlay stack. Wired to srv.popOverlay by Run.
	popOverlayFn func(id session.ClientID)

	// pushPaneOverlayFn, if non-nil, registers a PaneMode for the given client
	// and pane. Wired to srv.pushPaneOverlay by Run.
	pushPaneOverlayFn func(id session.ClientID, paneID session.PaneID, mode modes.PaneMode)

	// popPaneOverlayFn, if non-nil, removes the PaneMode for the given client
	// and pane. Wired to srv.popPaneOverlay by Run.
	popPaneOverlayFn func(id session.ClientID, paneID session.PaneID)

	// scrollViewportFn, if non-nil, shifts the viewport offset of the named
	// client by (dx, dy) cells and triggers a redraw. Wired to
	// srv.scrollViewport by Run.
	scrollViewportFn func(id session.ClientID, dx, dy int)
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
	_, _, p, err := m.findPane(paneID)
	if err != nil {
		return "", err
	}
	content, err := p.CaptureContent(history)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (m *serverMutator) RespawnPane(paneID int, shell string, kill bool, keepHistory bool) error {
	sess, _, p, err := m.findPane(paneID)
	if err != nil {
		return err
	}

	// Check whether the pane's process is still alive.
	pid := p.ShellPID()
	if pid > 0 {
		proc, findErr := os.FindProcess(pid)
		alive := findErr == nil && proc.Signal(syscall.Signal(0)) == nil
		if alive && !kill {
			return fmt.Errorf("pane still active")
		}
	}

	if !keepHistory {
		p.ClearHistory()
	}

	if err := p.Respawn(shell); err != nil {
		return fmt.Errorf("respawn-pane: %w", err)
	}

	m.markSessionDirty(sess)
	return nil
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
	sess, ok := m.state.Sessions[session.SessionID(sessionID)]
	if !ok {
		return fmt.Errorf("move-window: session %q not found", sessionID)
	}

	// Find the winlink for the window to move.
	srcIdx := -1
	for i, wl := range sess.Windows {
		if wl.Window.ID == session.WindowID(windowID) {
			srcIdx = i
			break
		}
	}
	if srcIdx < 0 {
		return fmt.Errorf("move-window: window %q not found in session %q", windowID, sessionID)
	}

	// If no explicit destination, leave the window in place.
	if newIndex == 0 {
		return nil
	}

	// Remove the winlink from its current position.
	wl := sess.Windows[srcIdx]
	sess.Windows = append(sess.Windows[:srcIdx], sess.Windows[srcIdx+1:]...)

	// Determine the target display index.
	var targetIdx int
	if newIndex < 0 {
		// Append after the last existing window.
		if len(sess.Windows) > 0 {
			targetIdx = sess.Windows[len(sess.Windows)-1].Index + 1
		} else {
			targetIdx = 1
		}
	} else {
		targetIdx = newIndex
	}

	// Shift all windows with index >= targetIdx up by one to make room.
	for _, existing := range sess.Windows {
		if existing.Index >= targetIdx {
			existing.Index++
		}
	}

	// Update the moved window's display index.
	wl.Index = targetIdx

	// Insert in sorted order by index.
	pos := len(sess.Windows)
	for i, existing := range sess.Windows {
		if existing.Index > targetIdx {
			pos = i
			break
		}
	}
	sess.Windows = append(sess.Windows[:pos], append([]*session.Winlink{wl}, sess.Windows[pos:]...)...)

	// Keep Current pointer valid.
	if sess.Current == nil && len(sess.Windows) > 0 {
		sess.Current = sess.Windows[0]
	}

	m.RunHook("after-move-window")
	return nil
}

func (m *serverMutator) SwapWindows(sessionID, aWindowID, bWindowID string) error {
	sess, ok := m.state.Sessions[session.SessionID(sessionID)]
	if !ok {
		return fmt.Errorf("swap-window: session %q not found", sessionID)
	}

	var wlA, wlB *session.Winlink
	for _, wl := range sess.Windows {
		switch wl.Window.ID {
		case session.WindowID(aWindowID):
			wlA = wl
		case session.WindowID(bWindowID):
			wlB = wl
		}
	}
	if wlA == nil {
		return fmt.Errorf("swap-window: window %q not found in session %q", aWindowID, sessionID)
	}
	if wlB == nil {
		return fmt.Errorf("swap-window: window %q not found in session %q", bWindowID, sessionID)
	}

	// Exchange display indices.
	wlA.Index, wlB.Index = wlB.Index, wlA.Index

	// Re-sort the Windows slice to reflect the new order.
	for i := 1; i < len(sess.Windows); i++ {
		for j := i; j > 0 && sess.Windows[j].Index < sess.Windows[j-1].Index; j-- {
			sess.Windows[j], sess.Windows[j-1] = sess.Windows[j-1], sess.Windows[j]
		}
	}

	m.RunHook("after-swap-window")
	return nil
}

func (m *serverMutator) FindWindow(sessionID, pattern string) (command.WindowView, error) {
	sess, ok := m.state.Sessions[session.SessionID(sessionID)]
	if !ok {
		return command.WindowView{}, fmt.Errorf("find-window: session %q not found", sessionID)
	}
	for _, wl := range sess.Windows {
		if strings.Contains(wl.Window.Name, pattern) {
			return toWindowView(wl), nil
		}
	}
	return command.WindowView{}, fmt.Errorf("find-window: no window matching %q", pattern)
}

// ─── Pane movement ───────────────────────────────────────────────────────────

func (m *serverMutator) SwapPane(sessionID, windowID string, paneA, paneB int) error {
	idA := session.PaneID(paneA)
	idB := session.PaneID(paneB)
	if idA == idB {
		return nil
	}

	win, err := m.findWindow(sessionID, windowID)
	if err != nil {
		return fmt.Errorf("swap-pane: %w", err)
	}
	if _, ok := win.Panes[idA]; !ok {
		return fmt.Errorf("swap-pane: pane %d not found in window %q", paneA, windowID)
	}
	if _, ok := win.Panes[idB]; !ok {
		return fmt.Errorf("swap-pane: pane %d not found in window %q", paneB, windowID)
	}

	win.Layout.SwapLeaves(idA, idB)

	// Notify both PTYs of any dimension change.
	for _, id := range []session.PaneID{idA, idB} {
		p := win.Panes[id]
		r := win.Layout.Rect(id)
		if r.Width > 0 && r.Height > 0 {
			_ = p.Resize(r.Width, r.Height)
		}
	}

	m.RunHook("after-swap-pane")
	return nil
}

func (m *serverMutator) BreakPane(sessionID, windowID string, paneID int) (command.WindowView, error) {
	srcID := session.PaneID(paneID)

	srcWin, err := m.findWindow(sessionID, windowID)
	if err != nil {
		return command.WindowView{}, fmt.Errorf("break-pane: %w", err)
	}
	srcPane, ok := srcWin.Panes[srcID]
	if !ok {
		return command.WindowView{}, fmt.Errorf("break-pane: pane %d not found in window %q", paneID, windowID)
	}

	sess, ok := m.state.Sessions[session.SessionID(sessionID)]
	if !ok {
		return command.WindowView{}, fmt.Errorf("break-pane: session %q not found", sessionID)
	}

	// Remove the pane from the source window's layout and pane map.
	srcWin.RemovePane(srcID)
	srcWin.Layout.Close(srcID)

	// Create a new window that will hold just this pane.
	m.nextWindowID++
	newWinID := session.WindowID(fmt.Sprintf("w%d", m.nextWindowID))
	newWin := session.NewWindow(newWinID, fmt.Sprintf("window%d", m.nextWindowID), sess.Options)

	// Size the new window to match the attached client, or fall back to 80×24.
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

	newWin.AddPane(srcID, srcPane)
	newWin.Layout = layout.New(cols, rows, srcID)

	// Resize the moved pane to fill the new window.
	_ = srcPane.Resize(cols, rows)

	// Rebalance remaining panes in the source window.
	for id, p := range srcWin.Panes {
		r := srcWin.Layout.Rect(id)
		if r.Width > 0 && r.Height > 0 {
			_ = p.Resize(r.Width, r.Height)
		}
	}

	// Add the new window to the session before potentially killing the source,
	// so the session is never left windowless.
	wl := sess.AddWindow(newWin)
	m.notifyAll(control.WindowAddEvent{WindowID: string(newWinID)})

	// If the source window is now empty, remove it.
	if len(srcWin.Panes) == 0 {
		_ = m.KillWindow(sessionID, windowID)
	}

	m.RunHook("after-break-pane")
	return toWindowView(wl), nil
}

func (m *serverMutator) JoinPane(srcSessionID, srcWindowID string, srcPaneID int, dstSessionID, dstWindowID string) error {
	srcID := session.PaneID(srcPaneID)

	srcWin, err := m.findWindow(srcSessionID, srcWindowID)
	if err != nil {
		return fmt.Errorf("join-pane: %w", err)
	}
	srcPane, ok := srcWin.Panes[srcID]
	if !ok {
		return fmt.Errorf("join-pane: pane %d not found in window %q", srcPaneID, srcWindowID)
	}
	dstWin, err := m.findWindow(dstSessionID, dstWindowID)
	if err != nil {
		return fmt.Errorf("join-pane: %w", err)
	}

	// Remove the pane from the source window.
	srcWin.RemovePane(srcID)
	srcWin.Layout.Close(srcID)

	// Insert the pane into the destination window by splitting the active pane.
	newID := dstWin.Layout.Split(dstWin.Active, layout.Vertical)
	dstWin.AddPane(newID, srcPane)

	// Resize the moved pane to its new slot.
	r := dstWin.Layout.Rect(newID)
	if r.Width > 0 && r.Height > 0 {
		_ = srcPane.Resize(r.Width, r.Height)
	}

	// Rebalance remaining panes in the source window.
	for id, p := range srcWin.Panes {
		r := srcWin.Layout.Rect(id)
		if r.Width > 0 && r.Height > 0 {
			_ = p.Resize(r.Width, r.Height)
		}
	}

	// If the source window is now empty, remove it.
	if len(srcWin.Panes) == 0 {
		_ = m.KillWindow(srcSessionID, srcWindowID)
	}

	m.RunHook("after-join-pane")
	return nil
}

func (m *serverMutator) MovePane(srcSessionID, srcWindowID string, srcPaneID int, dstSessionID, dstWindowID string) error {
	srcID := session.PaneID(srcPaneID)

	srcWin, err := m.findWindow(srcSessionID, srcWindowID)
	if err != nil {
		return fmt.Errorf("move-pane: %w", err)
	}
	srcPane, ok := srcWin.Panes[srcID]
	if !ok {
		return fmt.Errorf("move-pane: pane %d not found in window %q", srcPaneID, srcWindowID)
	}
	dstWin, err := m.findWindow(dstSessionID, dstWindowID)
	if err != nil {
		return fmt.Errorf("move-pane: %w", err)
	}

	// Remove the pane from the source window.
	srcWin.RemovePane(srcID)
	srcWin.Layout.Close(srcID)

	// Insert the pane into the destination window by splitting the active pane.
	newID := dstWin.Layout.Split(dstWin.Active, layout.Vertical)
	dstWin.AddPane(newID, srcPane)

	// Resize the moved pane to its new slot.
	r := dstWin.Layout.Rect(newID)
	if r.Width > 0 && r.Height > 0 {
		_ = srcPane.Resize(r.Width, r.Height)
	}

	// Resize all panes in the source window to account for the removed pane.
	for id, p := range srcWin.Panes {
		r := srcWin.Layout.Rect(id)
		if r.Width > 0 && r.Height > 0 {
			_ = p.Resize(r.Width, r.Height)
		}
	}

	// If the source window is now empty, remove it.
	if len(srcWin.Panes) == 0 {
		_ = m.KillWindow(srcSessionID, srcWindowID)
	}

	m.RunHook("after-move-pane")
	return nil
}

func (m *serverMutator) SlicePane(sessionID, windowID string, paneID int) (command.PaneView, error) {
	win, err := m.findWindow(sessionID, windowID)
	if err != nil {
		return command.PaneView{}, fmt.Errorf("slice-pane: %w", err)
	}

	targetID := session.PaneID(paneID)
	if _, ok := win.Panes[targetID]; !ok {
		return command.PaneView{}, fmt.Errorf("slice-pane: pane %d not found in window %q", paneID, windowID)
	}

	newPaneID := win.Layout.Split(targetID, layout.Vertical)

	p, err := m.newPane(pane.Config{ID: newPaneID})
	if err != nil {
		return command.PaneView{}, fmt.Errorf("slice-pane: creating pane: %w", err)
	}

	win.AddPane(newPaneID, p)

	// Notify PTY of dimension changes for all panes in the window.
	for id, wp := range win.Panes {
		r := win.Layout.Rect(id)
		if r.Width > 0 && r.Height > 0 {
			_ = wp.Resize(r.Width, r.Height)
		}
	}

	if m.watchPane != nil {
		m.watchPane(int(newPaneID))
	}

	m.RunHook("after-slice-pane")
	return command.PaneView{
		ID:    int(newPaneID),
		Title: p.Title(),
	}, nil
}

func (m *serverMutator) RespawnWindow(sessionID, windowID, shell, dir string, kill bool, keepHistory bool) error {
	sess, ok := m.state.Sessions[session.SessionID(sessionID)]
	if !ok {
		return fmt.Errorf("respawn-window: session %q not found", sessionID)
	}

	var win *session.Window
	for _, wl := range sess.Windows {
		if wl.Window.ID == session.WindowID(windowID) {
			win = wl.Window
			break
		}
	}
	if win == nil {
		return fmt.Errorf("respawn-window: window %q not found in session %q", windowID, sessionID)
	}

	// If -k is not set, fail early if any pane's process is still alive.
	if !kill {
		for _, p := range win.Panes {
			pid := p.ShellPID()
			if pid > 0 {
				proc, findErr := os.FindProcess(pid)
				alive := findErr == nil && proc.Signal(syscall.Signal(0)) == nil
				if alive {
					return fmt.Errorf("pane still active")
				}
			}
		}
	}

	// Respawn each pane in the window.
	for _, p := range win.Panes {
		if !keepHistory {
			p.ClearHistory()
		}
		if err := p.Respawn(shell); err != nil {
			return fmt.Errorf("respawn-window: %w", err)
		}
	}

	m.markSessionDirty(sess)
	return nil
}

// ─── Pane pipe / clear operations ────────────────────────────────────────────

// pipablePane is the subset of [pane.Pane] required by PipePane.
// The concrete *pane.pane satisfies this; test fakes that need to support
// piping must implement it too.
type pipablePane interface {
	HasPipe() bool
	AttachPipe(shellCmd string) error
	DetachPipe() error
}

func (m *serverMutator) PipePane(paneID int, shellCmd string, inFlag, outFlag, onceFlag bool) error {
	_, _, p, err := m.findPane(paneID)
	if err != nil {
		return fmt.Errorf("pipe-pane: %w", err)
	}

	pp, ok := p.(pipablePane)
	if !ok {
		return fmt.Errorf("pipe-pane: pane does not support piping")
	}

	// -o flag: only attach if the pane is not already piped.
	if onceFlag && pp.HasPipe() {
		return nil
	}

	// Stop any existing pipe (also handles toggle-off when shellCmd is empty).
	if err := pp.DetachPipe(); err != nil {
		return fmt.Errorf("pipe-pane: stop existing pipe: %w", err)
	}

	// Empty shellCmd means "disable pipe" (toggle behaviour).
	if shellCmd == "" {
		return nil
	}

	if err := pp.AttachPipe(shellCmd); err != nil {
		return fmt.Errorf("pipe-pane: %w", err)
	}
	return nil
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

// ─── Overlay lifecycle helpers ────────────────────────────────────────────────

// PushClientOverlay pushes ov onto the named client's overlay stack and
// triggers a redraw. It is the low-level helper called by Enter* methods.
func (m *serverMutator) PushClientOverlay(clientID string, ov modes.ClientOverlay) {
	if m.pushOverlayFn != nil {
		m.pushOverlayFn(session.ClientID(clientID), ov)
	}
}

// PopClientOverlay removes the topmost overlay from the named client's stack,
// calls Close on it, and triggers a redraw.
func (m *serverMutator) PopClientOverlay(clientID string) {
	if m.popOverlayFn != nil {
		m.popOverlayFn(session.ClientID(clientID))
	}
}

// ─── Mode entry mutations ─────────────────────────────────────────────────────

// snapshotScrollback implements copy.Scrollback using the pane's live Snapshot.
// Each call to Lines() returns a fresh snapshot from the underlying pane.
type snapshotScrollback struct {
	p session.Pane
}

func (s *snapshotScrollback) Lines() []copymode.Line {
	grid := s.p.Snapshot()
	if grid.Rows == 0 || grid.Cols == 0 {
		return nil
	}
	lines := make([]copymode.Line, grid.Rows)
	for row := 0; row < grid.Rows; row++ {
		line := make(copymode.Line, grid.Cols)
		for col := 0; col < grid.Cols; col++ {
			c := grid.Cells[row*grid.Cols+col]
			line[col] = modes.Cell{
				Char:  c.Char,
				Fg:    modes.Color(c.Fg),
				Bg:    modes.Color(c.Bg),
				Attrs: uint8(c.Attrs),
				FgR:   c.FgR,
				FgG:   c.FgG,
				FgB:   c.FgB,
				BgR:   c.BgR,
				BgG:   c.BgG,
				BgB:   c.BgB,
			}
		}
		lines[row] = line
	}
	return lines
}

func (s *snapshotScrollback) Width() int {
	return s.p.Snapshot().Cols
}

func (s *snapshotScrollback) Height() int {
	return s.p.Snapshot().Rows
}

func (m *serverMutator) EnterCopyMode(clientID string, _ bool) error {
	client, ok := m.state.Clients[session.ClientID(clientID)]
	if !ok {
		return fmt.Errorf("copy-mode: client %q not found", clientID)
	}
	if client.Session == nil || client.Session.Current == nil {
		return fmt.Errorf("copy-mode: client %q has no active window", clientID)
	}
	win := client.Session.Current.Window
	p, ok := win.Panes[win.Active]
	if !ok {
		return fmt.Errorf("copy-mode: no active pane in window %q", win.ID)
	}
	sb := &snapshotScrollback{p: p}
	mode := copymode.New(sb)
	if m.pushPaneOverlayFn != nil {
		m.pushPaneOverlayFn(session.ClientID(clientID), win.Active, mode)
	}
	return nil
}

// treeClientOverlay wraps a treemode.Mode as a modes.ClientOverlay that
// covers the full client screen.
type treeClientOverlay struct {
	mode *treemode.Mode
	rows int
	cols int
}

func (o *treeClientOverlay) Rect() modes.Rect {
	return modes.Rect{X: 0, Y: 0, Width: o.cols, Height: o.rows}
}

func (o *treeClientOverlay) Render(dst []modes.Cell) {
	canvas := &gridCanvas{rows: o.rows, cols: o.cols, cells: dst}
	o.mode.Render(canvas)
}

func (o *treeClientOverlay) Key(k keys.Key) modes.Outcome { return o.mode.Key(k) }
func (o *treeClientOverlay) Mouse(ev keys.MouseEvent) modes.Outcome { return o.mode.Mouse(ev) }
func (o *treeClientOverlay) CaptureFocus() bool { return true }
func (o *treeClientOverlay) Close()             { o.mode.Close() }

func (m *serverMutator) EnterChooseTree(clientID, sessionID, windowID string) error {
	client, ok := m.state.Clients[session.ClientID(clientID)]
	if !ok {
		return fmt.Errorf("choose-tree: client %q not found", clientID)
	}

	// Build a tree snapshot from all sessions, windows, and panes.
	var nodes []treemode.TreeNode
	for _, sess := range m.state.Sessions {
		sessNode := treemode.TreeNode{
			Kind: treemode.KindSession,
			ID:   "s:" + string(sess.ID),
			Name: sess.Name,
		}
		for _, wl := range sess.Windows {
			win := wl.Window
			winNode := treemode.TreeNode{
				Kind: treemode.KindWindow,
				ID:   "w:" + string(sess.ID) + ":" + string(win.ID),
				Name: fmt.Sprintf("%d: %s", wl.Index, win.Name),
			}
			for paneID := range win.Panes {
				paneNode := treemode.TreeNode{
					Kind: treemode.KindPane,
					ID:   fmt.Sprintf("p:%s:%s:%d", sess.ID, win.ID, int(paneID)),
					Name: fmt.Sprintf("%%%d", int(paneID)),
				}
				winNode.Children = append(winNode.Children, paneNode)
			}
			sessNode.Children = append(sessNode.Children, winNode)
		}
		nodes = append(nodes, sessNode)
	}

	// Snapshot the client terminal size for the overlay rectangle.
	rows, cols := 24, 80
	if client.Size.Rows > 0 {
		rows = client.Size.Rows
	}
	if client.Size.Cols > 0 {
		cols = client.Size.Cols
	}

	// onSelect is called when the user presses Enter on a tree entry.
	onSelect := func(id string) {
		parts := strings.SplitN(id, ":", 4)
		if len(parts) == 0 {
			return
		}
		switch parts[0] {
		case "s":
			if len(parts) < 2 {
				return
			}
			m.SwitchClient(clientID, parts[1]) //nolint:errcheck
		case "w":
			if len(parts) < 3 {
				return
			}
			sID, wID := parts[1], parts[2]
			if client.Session == nil || string(client.Session.ID) != sID {
				m.SwitchClient(clientID, sID) //nolint:errcheck
			}
			m.SelectWindow(sID, wID) //nolint:errcheck
		case "p":
			if len(parts) < 4 {
				return
			}
			sID, wID, pIDStr := parts[1], parts[2], parts[3]
			pID, err := strconv.Atoi(pIDStr)
			if err != nil {
				return
			}
			if client.Session == nil || string(client.Session.ID) != sID {
				m.SwitchClient(clientID, sID) //nolint:errcheck
			}
			m.SelectWindow(sID, wID) //nolint:errcheck
			m.SelectPane(sID, wID, pID) //nolint:errcheck
		}
	}

	mode := treemode.New(nodes, onSelect, nil)
	overlay := &treeClientOverlay{mode: mode, rows: rows, cols: cols}
	m.PushClientOverlay(clientID, overlay)
	return nil
}

func (m *serverMutator) EnterCustomizeMode(clientID string) error {
	client, ok := m.state.Clients[session.ClientID(clientID)]
	if !ok {
		return fmt.Errorf("customize-mode: client %q not found", clientID)
	}

	// Snapshot client terminal size for the overlay rectangle.
	rows, cols := 24, 80
	if client.Size.Rows > 0 {
		rows = client.Size.Rows
	}
	if client.Size.Cols > 0 {
		cols = client.Size.Cols
	}

	// Collect options: server first, then active session and window.
	var opts []modes.CustomizeOptionEntry
	if m.state.Options != nil {
		m.state.Options.Each(func(name string, val options.Value) {
			opts = append(opts, modes.CustomizeOptionEntry{
				Scope: "server",
				Name:  name,
				Value: val.String(),
			})
		})
	}
	if client.Session != nil {
		sessScope := "session:" + string(client.Session.ID)
		if client.Session.Options != nil {
			client.Session.Options.Each(func(name string, val options.Value) {
				opts = append(opts, modes.CustomizeOptionEntry{
					Scope: sessScope,
					Name:  name,
					Value: val.String(),
				})
			})
		}
		if client.Session.Current != nil && client.Session.Current.Window.Options != nil {
			winScope := "window:" + string(client.Session.ID) + ":" + string(client.Session.Current.Window.ID)
			client.Session.Current.Window.Options.Each(func(name string, val options.Value) {
				opts = append(opts, modes.CustomizeOptionEntry{
					Scope: winScope,
					Name:  name,
					Value: val.String(),
				})
			})
		}
	}

	// Collect key bindings from all tables.
	var bindings []modes.CustomizeBindingEntry
	for _, tableName := range m.state.KeyTables.Names() {
		t, ok := m.state.KeyTables.Get(tableName)
		if !ok {
			continue
		}
		t.Each(func(k keys.Key, cmd keys.BoundCommand) {
			bindings = append(bindings, modes.CustomizeBindingEntry{
				Table:   tableName,
				Key:     k.String(),
				Command: fmt.Sprintf("%v", cmd),
			})
		})
	}

	setOption := func(scope, name, value string) error {
		return m.SetOption(scope, name, value)
	}
	bindKeyFn := func(table, key, cmd string) error {
		return m.BindKey(table, key, cmd)
	}

	rect := modes.Rect{X: 0, Y: 0, Width: cols, Height: rows}
	overlay := modes.NewCustomizeOverlay(rect, opts, bindings, setOption, bindKeyFn)
	m.PushClientOverlay(clientID, overlay)
	return nil
}

// chooseBufferClientOverlay wraps a treemode.Mode for the choose-buffer
// command. It adds buffer-delete ('d') support on top of the standard tree
// mode key handling.
type chooseBufferClientOverlay struct {
	mode     *treemode.Mode
	mutator  *serverMutator
	rows     int
	cols     int
	items    []treemode.ChooserItem
	onSelect func(string)
}

func (o *chooseBufferClientOverlay) Rect() modes.Rect {
	return modes.Rect{X: 0, Y: 0, Width: o.cols, Height: o.rows}
}

func (o *chooseBufferClientOverlay) Render(dst []modes.Cell) {
	canvas := &gridCanvas{rows: o.rows, cols: o.cols, cells: dst}
	o.mode.Render(canvas)
}

func (o *chooseBufferClientOverlay) Key(k keys.Key) modes.Outcome {
	// 'd' in normal mode deletes the currently selected buffer.
	if k.Code == keys.KeyCode('d') && !o.mode.Searching() {
		selected := o.mode.SelectedID()
		if selected != "" {
			o.mutator.DeleteBuffer(selected) //nolint:errcheck
			for i, item := range o.items {
				if item.Value == selected {
					o.items = append(o.items[:i], o.items[i+1:]...)
					break
				}
			}
			o.mode = treemode.NewChooser(o.items, o.onSelect, false)
		}
		return modes.Consumed()
	}
	return o.mode.Key(k)
}

func (o *chooseBufferClientOverlay) Mouse(ev keys.MouseEvent) modes.Outcome {
	return o.mode.Mouse(ev)
}

func (o *chooseBufferClientOverlay) CaptureFocus() bool { return true }
func (o *chooseBufferClientOverlay) Close()             { o.mode.Close() }

func (m *serverMutator) EnterChooseBuffer(clientID, windowID string, items []command.ChooserItem, template string) error {
	client, ok := m.state.Clients[session.ClientID(clientID)]
	if !ok {
		return fmt.Errorf("choose-buffer: client %q not found", clientID)
	}

	// Snapshot the client terminal size for the overlay rectangle.
	rows, cols := 24, 80
	if client.Size.Rows > 0 {
		rows = client.Size.Rows
	}
	if client.Size.Cols > 0 {
		cols = client.Size.Cols
	}

	// Determine the active pane to paste into.
	var activePaneID int
	if client.Session != nil && client.Session.Current != nil {
		activePaneID = int(client.Session.Current.Window.Active)
	}

	// Convert command.ChooserItem to treemode.ChooserItem.
	treeItems := make([]treemode.ChooserItem, len(items))
	for i, it := range items {
		treeItems[i] = treemode.ChooserItem{
			Display: it.Display,
			Preview: it.Preview,
			Value:   it.Value,
		}
	}

	onSelect := func(name string) {
		m.PasteBuffer(name, activePaneID) //nolint:errcheck
	}

	overlay := &chooseBufferClientOverlay{
		mutator:  m,
		rows:     rows,
		cols:     cols,
		items:    treeItems,
		onSelect: onSelect,
	}
	overlay.mode = treemode.NewChooser(treeItems, onSelect, false)

	m.PushClientOverlay(clientID, overlay)
	return nil
}

// chooseClientClientOverlay wraps a treemode.Mode for the choose-client
// command. It adds client-detach ('d') support on top of the standard tree
// mode key handling.
type chooseClientClientOverlay struct {
	mode        *treemode.Mode
	mutator     *serverMutator
	callerID    string // the client that opened the overlay
	rows        int
	cols        int
	items       []treemode.ChooserItem
	onSelect    func(string)
}

func (o *chooseClientClientOverlay) Rect() modes.Rect {
	return modes.Rect{X: 0, Y: 0, Width: o.cols, Height: o.rows}
}

func (o *chooseClientClientOverlay) Render(dst []modes.Cell) {
	canvas := &gridCanvas{rows: o.rows, cols: o.cols, cells: dst}
	o.mode.Render(canvas)
}

func (o *chooseClientClientOverlay) Key(k keys.Key) modes.Outcome {
	// 'd' in normal mode detaches the currently selected client.
	if k.Code == keys.KeyCode('d') && !o.mode.Searching() {
		selected := o.mode.SelectedID()
		if selected != "" {
			o.mutator.DetachClient(selected) //nolint:errcheck
			for i, item := range o.items {
				if item.Value == selected {
					o.items = append(o.items[:i], o.items[i+1:]...)
					break
				}
			}
			o.mode = treemode.NewChooser(o.items, o.onSelect, false)
		}
		return modes.Consumed()
	}
	return o.mode.Key(k)
}

func (o *chooseClientClientOverlay) Mouse(ev keys.MouseEvent) modes.Outcome {
	return o.mode.Mouse(ev)
}

func (o *chooseClientClientOverlay) CaptureFocus() bool { return true }
func (o *chooseClientClientOverlay) Close()             { o.mode.Close() }

func (m *serverMutator) EnterChooseClient(clientID, windowID string, items []command.ChooserItem, template string) error {
	client, ok := m.state.Clients[session.ClientID(clientID)]
	if !ok {
		return fmt.Errorf("choose-client: client %q not found", clientID)
	}

	// Snapshot the client terminal size for the overlay rectangle.
	rows, cols := 24, 80
	if client.Size.Rows > 0 {
		rows = client.Size.Rows
	}
	if client.Size.Cols > 0 {
		cols = client.Size.Cols
	}

	// Convert command.ChooserItem to treemode.ChooserItem.
	treeItems := make([]treemode.ChooserItem, len(items))
	for i, it := range items {
		treeItems[i] = treemode.ChooserItem{
			Display: it.Display,
			Preview: it.Preview,
			Value:   it.Value,
		}
	}

	// onSelect switches the calling client to the selected client's session.
	onSelect := func(selectedClientID string) {
		target, ok := m.state.Clients[session.ClientID(selectedClientID)]
		if !ok {
			return
		}
		if target.Session == nil {
			return
		}
		m.SwitchClient(clientID, string(target.Session.ID)) //nolint:errcheck
	}

	overlay := &chooseClientClientOverlay{
		mutator:  m,
		callerID: clientID,
		rows:     rows,
		cols:     cols,
		items:    treeItems,
		onSelect: onSelect,
	}
	overlay.mode = treemode.NewChooser(treeItems, onSelect, false)

	m.PushClientOverlay(clientID, overlay)
	return nil
}

// clockPaneMode wraps clock.Mode and stops a background ticker on Close.
type clockPaneMode struct {
	mode   *clockmode.Mode
	stopFn func()
}

func (c *clockPaneMode) Render(dst modes.Canvas)                  { c.mode.Render(dst) }
func (c *clockPaneMode) Key(k keys.Key) modes.Outcome             { return c.mode.Key(k) }
func (c *clockPaneMode) Mouse(ev keys.MouseEvent) modes.Outcome   { return c.mode.Mouse(ev) }
func (c *clockPaneMode) Close() {
	if c.stopFn != nil {
		c.stopFn()
	}
	c.mode.Close()
}

func (m *serverMutator) EnterClockMode(clientID string, paneID int) error {
	client, ok := m.state.Clients[session.ClientID(clientID)]
	if !ok {
		return fmt.Errorf("clock-mode: client %q not found", clientID)
	}
	if client.Session == nil || client.Session.Current == nil {
		return fmt.Errorf("clock-mode: client %q has no active window", clientID)
	}
	win := client.Session.Current.Window
	targetPaneID := session.PaneID(paneID)
	if paneID <= 0 {
		targetPaneID = win.Active
	}
	if _, ok := win.Panes[targetPaneID]; !ok {
		return fmt.Errorf("clock-mode: pane %d not found", paneID)
	}

	clockModeInst := clockmode.New(nil)

	// Start a 1-second ticker that triggers a redraw so the clock updates.
	ticker := time.NewTicker(time.Second)
	stop := make(chan struct{})
	getConn := m.getConn
	markDirty := m.markDirty
	cid := session.ClientID(clientID)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if getConn != nil && markDirty != nil {
					if conn, ok := getConn(cid); ok {
						markDirty(conn)
					}
				}
			case <-stop:
				return
			}
		}
	}()

	paneMode := &clockPaneMode{
		mode:   clockModeInst,
		stopFn: func() { close(stop) },
	}

	if m.pushPaneOverlayFn != nil {
		m.pushPaneOverlayFn(cid, targetPaneID, paneMode)
	}
	return nil
}

func (m *serverMutator) DisplayPopup(clientID, cmd, title string, cols, rows int) error {
	client, ok := m.state.Clients[session.ClientID(clientID)]
	if !ok {
		return fmt.Errorf("display-popup: client %q not found", clientID)
	}

	// Use client terminal size for centering.
	clientRows, clientCols := 24, 80
	if client.Size.Rows > 0 {
		clientRows = client.Size.Rows
	}
	if client.Size.Cols > 0 {
		clientCols = client.Size.Cols
	}

	// Clamp popup dimensions to fit within the client terminal.
	if cols > clientCols {
		cols = clientCols
	}
	if rows > clientRows {
		rows = clientRows
	}

	// Center the popup.
	x := (clientCols - cols) / 2
	y := (clientRows - rows) / 2

	rect := modes.Rect{X: x, Y: y, Width: cols, Height: rows}

	newPaneFn := m.newPane
	factory := func(innerRows, innerCols int, command string) (popupmode.Pane, error) {
		if command == "" || newPaneFn == nil {
			return &nullPopupPane{}, nil
		}
		p, err := newPaneFn(pane.Config{ID: session.PaneID(0)})
		if err != nil {
			return nil, err
		}
		_ = p.Resize(innerCols, innerRows)
		return newSessionPaneAsPopupPane(p), nil
	}

	mode, err := popupmode.New(rect, cmd, false, factory)
	if err != nil {
		return fmt.Errorf("display-popup: %w", err)
	}

	m.PushClientOverlay(clientID, mode)
	return nil
}

// sessionPaneAsPopupPane adapts session.Pane to popup.Pane.
// session.Pane lacks SendMouse; we delegate to the underlying pane.Pane if
// the concrete type supports it, otherwise we silently ignore mouse events.
type sessionPaneAsPopupPane struct {
	session.Pane
	ms mouseSenderer
}

type mouseSenderer interface {
	SendMouse(ev keys.MouseEvent) error
}

func newSessionPaneAsPopupPane(p session.Pane) *sessionPaneAsPopupPane {
	a := &sessionPaneAsPopupPane{Pane: p}
	if ms, ok := p.(mouseSenderer); ok {
		a.ms = ms
	}
	return a
}

func (a *sessionPaneAsPopupPane) SendMouse(ev keys.MouseEvent) error {
	if a.ms != nil {
		return a.ms.SendMouse(ev)
	}
	return nil
}

// nullPopupPane is a no-op pane used when no subprocess command is given.
type nullPopupPane struct{}

func (n *nullPopupPane) Write(_ []byte) error                  { return nil }
func (n *nullPopupPane) SendKey(_ keys.Key) error              { return nil }
func (n *nullPopupPane) SendMouse(_ keys.MouseEvent) error     { return nil }
func (n *nullPopupPane) Resize(_, _ int) error                 { return nil }
func (n *nullPopupPane) Snapshot() pane.CellGrid               { return pane.CellGrid{} }
func (n *nullPopupPane) Close() error                          { return nil }

func (m *serverMutator) DisplayMenu(clientID string, items []command.MenuEntry) error {
	if _, ok := m.state.Clients[session.ClientID(clientID)]; !ok {
		return fmt.Errorf("display-menu: client %q not found", clientID)
	}

	store := m.store
	queue := m.queue
	mut := command.Mutator(m)

	menuItems := make([]menumode.MenuItem, len(items))
	for i, entry := range items {
		entry := entry
		var mnemonic rune
		if len(entry.Key) > 0 {
			mnemonic = []rune(entry.Key)[0]
		}
		isSeparator := entry.Label == "" && entry.Key == "" && entry.Command == ""
		onSelect := func() {
			if entry.Command == "" {
				return
			}
			cmds, err := parse.Parse(entry.Command)
			if err != nil || len(cmds) == 0 {
				return
			}
			for _, c := range cmds {
				c := c
				var nilClientView command.ClientView
				queue.EnqueueFunc(c.Name, func() {
					command.Dispatch(c.Name, c.Args, store, nilClientView, queue, mut)
				})
			}
		}
		menuItems[i] = menumode.MenuItem{
			Label:     entry.Label,
			Mnemonic:  mnemonic,
			Separator: isSeparator,
			Enabled:   !isSeparator,
			OnSelect:  onSelect,
		}
	}

	anchor := modes.Rect{X: 0, Y: 0}
	mode := menumode.New(anchor, menuItems)
	m.PushClientOverlay(clientID, mode)
	return nil
}

// displayPanesClientOverlay wraps a *displaypanes.Mode and intercepts
// SelectPaneCommand outcomes, calling onSelect and returning CloseMode so the
// server's overlay dispatch loop pops the overlay automatically.
type displayPanesClientOverlay struct {
	mode     *displaypanes.Mode
	onSelect func(paneNumber int)
}

func (d *displayPanesClientOverlay) Rect() modes.Rect                      { return d.mode.Rect() }
func (d *displayPanesClientOverlay) Render(dst []modes.Cell)               { d.mode.Render(dst) }
func (d *displayPanesClientOverlay) CaptureFocus() bool                    { return d.mode.CaptureFocus() }
func (d *displayPanesClientOverlay) Close()                                { d.mode.Close() }
func (d *displayPanesClientOverlay) Mouse(ev keys.MouseEvent) modes.Outcome { return d.mode.Mouse(ev) }

func (d *displayPanesClientOverlay) Key(k keys.Key) modes.Outcome {
	outcome := d.mode.Key(k)
	if outcome.Kind == modes.KindCommand {
		if cmd, ok := outcome.Cmd.(displaypanes.SelectPaneCommand); ok && d.onSelect != nil {
			d.onSelect(cmd.PaneNumber)
		}
		return modes.CloseMode()
	}
	return outcome
}

func (m *serverMutator) DisplayPanes(clientID string) error {
	client, ok := m.state.Clients[session.ClientID(clientID)]
	if !ok {
		return fmt.Errorf("display-panes: client %q not found", clientID)
	}
	if client.Session == nil {
		return fmt.Errorf("display-panes: client %q has no attached session", clientID)
	}
	if client.Session.Current == nil {
		return fmt.Errorf("display-panes: client %q has no current window", clientID)
	}

	win := client.Session.Current.Window
	sessID := string(client.Session.ID)
	winID := string(win.ID)

	// Collect visible panes from the layout tree, assigning numbers 0–9.
	var paneInfos []displaypanes.PaneInfo
	number := 0
	for leafID := range win.Layout.Leaves() {
		if number > 9 {
			break
		}
		r := win.Layout.Rect(leafID)
		if r.Width == 0 || r.Height == 0 {
			continue // hidden (e.g. behind a zoomed pane)
		}
		paneInfos = append(paneInfos, displaypanes.PaneInfo{
			ID:     fmt.Sprintf("%d", int(leafID)),
			Number: number,
			Bounds: modes.Rect{X: r.X, Y: r.Y, Width: r.Width, Height: r.Height},
		})
		number++
	}

	// Client terminal size for the overlay bounds.
	rows, cols := 24, 80
	if client.Size.Rows > 0 {
		rows = client.Size.Rows
	}
	if client.Size.Cols > 0 {
		cols = client.Size.Cols
	}
	bounds := modes.Rect{X: 0, Y: 0, Width: cols, Height: rows}

	// Read display-panes-time option (milliseconds).
	durationMs := 1000
	if m.state.Options != nil {
		if d, ok := m.state.Options.GetInt("display-panes-time"); ok {
			durationMs = d
		}
	}

	// onSelect resolves the pane number to a pane ID and calls SelectPane.
	onSelect := func(paneNumber int) {
		for _, pi := range paneInfos {
			if pi.Number == paneNumber {
				paneID, err := strconv.Atoi(pi.ID)
				if err == nil {
					m.SelectPane(sessID, winID, paneID) //nolint:errcheck
				}
				return
			}
		}
	}

	// scheduleTimeout arranges auto-dismissal after durationMs milliseconds.
	scheduleTimeout := func(dismiss func()) {
		time.AfterFunc(time.Duration(durationMs)*time.Millisecond, func() {
			dismiss()
			m.PopClientOverlay(clientID)
		})
	}

	mode := displaypanes.New(bounds, paneInfos, scheduleTimeout)
	overlay := &displayPanesClientOverlay{mode: mode, onSelect: onSelect}
	m.PushClientOverlay(clientID, overlay)
	return nil
}

func (m *serverMutator) CommandPrompt(clientID, promptStr, initialValue string) error {
	client, ok := m.state.Clients[session.ClientID(clientID)]
	if !ok {
		return fmt.Errorf("command-prompt: client %q not found", clientID)
	}

	rows, cols := 24, 80
	if client.Size.Rows > 0 {
		rows = client.Size.Rows
	}
	if client.Size.Cols > 0 {
		cols = client.Size.Cols
	}

	if promptStr == "" {
		promptStr = ":"
	}

	store := m.store
	queue := m.queue
	mut := command.Mutator(m)

	onConfirm := func(input string) {
		cmds, err := parse.Parse(input)
		if err != nil || len(cmds) == 0 {
			return
		}
		for _, c := range cmds {
			c := c
			var nilClientView command.ClientView
			queue.EnqueueFunc(c.Name, func() {
				command.Dispatch(c.Name, c.Args, store, nilClientView, queue, mut)
			})
		}
	}

	rect := modes.Rect{X: 0, Y: rows - 1, Width: cols, Height: 1}
	mode := promptmode.NewCommand(rect, promptmode.Config{
		Prompt:    promptStr,
		Initial:   initialValue,
		OnConfirm: onConfirm,
	})

	m.PushClientOverlay(clientID, mode)
	return nil
}

func (m *serverMutator) ConfirmBefore(clientID, prompt, cmd string) error {
	client, ok := m.state.Clients[session.ClientID(clientID)]
	if !ok {
		return fmt.Errorf("confirm-before: client %q not found", clientID)
	}

	rows, cols := 24, 80
	if client.Size.Rows > 0 {
		rows = client.Size.Rows
	}
	if client.Size.Cols > 0 {
		cols = client.Size.Cols
	}

	store := m.store
	queue := m.queue
	mut := command.Mutator(m)

	onYes := func() {
		cmds, err := parse.Parse(cmd)
		if err != nil || len(cmds) == 0 {
			return
		}
		for _, c := range cmds {
			c := c
			var nilClientView command.ClientView
			queue.EnqueueFunc(c.Name, func() {
				command.Dispatch(c.Name, c.Args, store, nilClientView, queue, mut)
			})
		}
	}

	displayPrompt := prompt + " (y/n)"
	rect := modes.Rect{X: 0, Y: rows - 1, Width: cols, Height: 1}
	mode := promptmode.NewConfirm(rect, displayPrompt, onYes, nil)

	m.PushClientOverlay(clientID, mode)
	return nil
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

// ─── refresh-client sub-features ─────────────────────────────────────────────

// parseFeatureFlags converts a comma-separated list of feature-flag names into
// a session.FeatureSet bitmask.  Unrecognised names are silently ignored.
func parseFeatureFlags(s string) session.FeatureSet {
	var fs session.FeatureSet
	for _, part := range strings.Split(s, ",") {
		switch strings.TrimSpace(strings.ToLower(part)) {
		case "256", "256colour", "256-colour", "256color":
			fs |= session.FeatureColour256
		case "rgb", "truecolour", "truecolor", "16m", "colour16m":
			fs |= session.FeatureColour16M
		case "mouse-sgr", "mousesgr", "mouse":
			fs |= session.FeatureMouseSGR
		case "overlap", "overlapping-windows":
			fs |= session.FeatureOverlap
		}
	}
	return fs
}

// SetClientFeatures parses featuresStr (comma-separated feature names) into a
// bitmask and stores it in the client's Features field.
func (m *serverMutator) SetClientFeatures(clientID, featuresStr string) error {
	c, ok := m.state.Clients[session.ClientID(clientID)]
	if !ok {
		return fmt.Errorf("refresh-client: client %q not found", clientID)
	}
	c.Features = parseFeatureFlags(featuresStr)
	return nil
}

// RequestClientClipboard sends an OSC 52 clipboard query sequence to the named
// client's terminal.  The terminal responds asynchronously; the response is
// stored in Client.ClipboardData when it is received.
func (m *serverMutator) RequestClientClipboard(clientID string) error {
	cc, ok := m.getConn(session.ClientID(clientID))
	if !ok {
		return fmt.Errorf("refresh-client: client %q not connected", clientID)
	}
	// OSC 52 ; c ; ? ST — request clipboard selection "c".
	query := proto.StdoutMsg{Data: []byte("\033]52;c;?\033\\")}
	return proto.WriteMsg(cc.netConn, proto.MsgStdout, query.Encode())
}

// AddClientSubscription registers a named notification subscription on the
// client.  When the event identified by notify fires, the server formats a
// message using format and delivers it to the client.
func (m *serverMutator) AddClientSubscription(clientID, name, notify, format string) error {
	c, ok := m.state.Clients[session.ClientID(clientID)]
	if !ok {
		return fmt.Errorf("refresh-client: client %q not found", clientID)
	}
	if c.Subscriptions == nil {
		c.Subscriptions = make(map[string]session.Subscription)
	}
	c.Subscriptions[name] = session.Subscription{Name: name, Notify: notify, Format: format}
	return nil
}

// ScrollClientViewport shifts the client's viewport offset by (dx, dy) cells
// and schedules a redraw.  Positive dy scrolls down; negative dy scrolls up.
func (m *serverMutator) ScrollClientViewport(clientID string, dx, dy int) error {
	if _, ok := m.state.Clients[session.ClientID(clientID)]; !ok {
		return fmt.Errorf("refresh-client: client %q not found", clientID)
	}
	if m.scrollViewportFn != nil {
		m.scrollViewportFn(session.ClientID(clientID), dx, dy)
	}
	return nil
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
