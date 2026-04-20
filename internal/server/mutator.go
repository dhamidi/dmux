package server

import (
	"fmt"
	"log"
	"os"

	"github.com/dhamidi/dmux/internal/command"
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/session"
)

// serverMutator wraps *session.Server and implements command.Mutator.
// Session, window, pane, and key-binding mutations are stubs pending
// their respective implementation layers; buffer mutations are fully
// wired to srv.Buffers.
type serverMutator struct {
	state         *session.Server
	nextSessionID uint64
	nextWindowID  uint64
	shutdown      func()
}

// newServerMutator returns a command.Mutator backed by state.
// shutdown is called by KillServer to trigger a graceful shutdown.
func newServerMutator(state *session.Server, shutdown func()) command.Mutator {
	return &serverMutator{state: state, shutdown: shutdown}
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
	return command.SessionView{
		ID:      string(id),
		Name:    name,
		Windows: []command.WindowView{},
		Current: -1,
	}, nil
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
	return m.state.AttachClient(c, session.SessionID(sessionID))
}

func (m *serverMutator) DetachClient(clientID string) error {
	return errStub("detach-client")
}

func (m *serverMutator) SwitchClient(clientID, sessionID string) error {
	return errStub("switch-client")
}

func (m *serverMutator) NewWindow(sessionID, name string) (command.WindowView, error) {
	return command.WindowView{}, errStub("new-window")
}

func (m *serverMutator) KillWindow(sessionID, windowID string) error {
	return errStub("kill-window")
}

func (m *serverMutator) RenameWindow(sessionID, windowID, name string) error {
	return errStub("rename-window")
}

func (m *serverMutator) SelectWindow(sessionID, windowID string) error {
	return errStub("select-window")
}

func (m *serverMutator) SplitWindow(sessionID, windowID string) (command.PaneView, error) {
	return command.PaneView{}, errStub("split-window")
}

func (m *serverMutator) KillPane(paneID int) error {
	return errStub("kill-pane")
}

func (m *serverMutator) SelectPane(sessionID, windowID string, paneID int) error {
	return errStub("select-pane")
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
	return errStub("unbind-key")
}

func (m *serverMutator) ListKeyBindings(table string) []command.KeyBinding {
	return nil
}

func (m *serverMutator) SetOption(scope, name, value string) error {
	return errStub("set-option")
}

func (m *serverMutator) UnsetOption(scope, name string) error {
	return errStub("unset-option")
}

func (m *serverMutator) ListOptions(scope string) []command.OptionEntry {
	return nil
}

func (m *serverMutator) KillServer() error {
	m.shutdown()
	return nil
}

func (m *serverMutator) DisplayMessage(clientID, msg string) error {
	return errStub("display-message")
}

func (m *serverMutator) SendKeys(paneID int, keys []string) error {
	return errStub("send-keys")
}

func (m *serverMutator) RunShell(cmd string, background bool) (string, error) {
	return "", errStub("run-shell")
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
	if _, ok := m.state.Buffers.GetNamed(name); !ok {
		return fmt.Errorf("paste-buffer: buffer %q not found", name)
	}
	// Key injection into a live pane is not yet wired.
	return errStub("paste-buffer (key injection)")
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
