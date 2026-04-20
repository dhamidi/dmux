package server

import (
	"fmt"
	"os"

	"github.com/dhamidi/dmux/internal/command"
	"github.com/dhamidi/dmux/internal/session"
)

// serverMutator wraps *session.Server and implements command.Mutator.
// Session, window, pane, and key-binding mutations are stubs pending
// their respective implementation layers; buffer mutations are fully
// wired to srv.Buffers.
type serverMutator struct {
	state *session.Server
}

// newServerMutator returns a command.Mutator backed by state.
func newServerMutator(state *session.Server) command.Mutator {
	return &serverMutator{state: state}
}

func errStub(method string) error {
	return fmt.Errorf("%s: not yet implemented", method)
}

func (m *serverMutator) NewSession(name string) (command.SessionView, error) {
	return command.SessionView{}, errStub("new-session")
}

func (m *serverMutator) KillSession(id string) error {
	return errStub("kill-session")
}

func (m *serverMutator) RenameSession(id, name string) error {
	return errStub("rename-session")
}

func (m *serverMutator) AttachClient(clientID, sessionID string) error {
	return errStub("attach-client")
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

func (m *serverMutator) BindKey(table, key, cmd string) error {
	return errStub("bind-key")
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
	return errStub("kill-server")
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
