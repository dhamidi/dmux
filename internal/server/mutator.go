package server

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
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
	// newPane creates a new pane from the given configuration.
	// Injected from Run (real PTY) or tests (fake).
	newPane func(cfg pane.Config) (session.Pane, error)
	// watchPane, if non-nil, is called after each new pane is created.
	// It starts a goroutine that fires pane-died when the process exits.
	watchPane func(paneID int)
}

// newServerMutator returns a command.Mutator backed by state.
// shutdown is called by KillServer to trigger a graceful shutdown.
// getConn and markDirty provide access to the live connection map for
// operations that need to write directly to a client's network connection.
// newPaneFn is the factory used to create new panes; pass a fake in tests.
func newServerMutator(
	state *session.Server,
	store command.Server,
	queue *command.Queue,
	shutdown func(),
	getConn func(session.ClientID) (*clientConn, bool),
	markDirty func(*clientConn),
	newPaneFn func(cfg pane.Config) (session.Pane, error),
	watchPaneFn func(paneID int),
) command.Mutator {
	return &serverMutator{
		state:     state,
		store:     store,
		queue:     queue,
		shutdown:  shutdown,
		getConn:   getConn,
		markDirty: markDirty,
		newPane:   newPaneFn,
		watchPane: watchPaneFn,
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
	m.RunHook("after-kill-session")
	return nil
}

func (m *serverMutator) RenameSession(id, name string) error {
	sess, ok := m.state.Sessions[session.SessionID(id)]
	if !ok {
		return fmt.Errorf("rename-session: session %q not found", id)
	}
	sess.Name = name
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
	win.Active = targetID
	return nil
}

func (m *serverMutator) ResizePane(paneID int, direction string, amount int) error {
	return errStub("resize-pane")
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
