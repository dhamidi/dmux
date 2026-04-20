package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/session"
	"time"
)

// capturingPane is a session.Pane that returns preset content from CaptureContent.
type capturingPane struct {
	id      session.PaneID
	content []byte
	history bool // records whether history was requested
}

func (c *capturingPane) Title() string                    { return "capturing" }
func (c *capturingPane) Write(data []byte) error          { return nil }
func (c *capturingPane) SendKey(key keys.Key) error       { return nil }
func (c *capturingPane) Resize(cols, rows int) error      { return nil }
func (c *capturingPane) Snapshot() pane.CellGrid          { return pane.CellGrid{} }
func (c *capturingPane) CaptureContent(history bool) ([]byte, error) {
	c.history = history
	return c.content, nil
}
func (c *capturingPane) Respawn(shell string) error { return nil }
func (c *capturingPane) Close() error               { return nil }
func (c *capturingPane) ShellPID() int              { return 0 }
func (c *capturingPane) LastOutputAt() time.Time    { return time.Time{} }
func (c *capturingPane) ConsumeBell() bool          { return false }
func (c *capturingPane) ClearHistory()              {}
func (c *capturingPane) ClearScreen() error         { return nil }

func newTestMutatorWithCapturingPane(content string) (*serverMutator, *capturingPane) {
	state := session.NewServer()
	cp := &capturingPane{content: []byte(content)}
	m := &serverMutator{
		state:    state,
		shutdown: func() {},
		newPane: func(cfg pane.Config) (session.Pane, error) {
			cp.id = cfg.ID
			return cp, nil
		},
	}
	sv, _ := m.NewSession("s1")
	_, _ = m.NewWindow(sv.ID, "w1")
	return m, cp
}

func TestCapturePane_ReturnsContent(t *testing.T) {
	expected := "hello world\n"
	m, cp := newTestMutatorWithCapturingPane(expected)

	got, err := m.CapturePane(int(cp.id), false)
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	if got != expected {
		t.Errorf("content = %q, want %q", got, expected)
	}
}

func TestCapturePane_PassesHistoryFlag(t *testing.T) {
	m, cp := newTestMutatorWithCapturingPane("some content\n")

	_, err := m.CapturePane(int(cp.id), true)
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	if !cp.history {
		t.Error("history flag was not passed to CaptureContent")
	}
}

func TestCapturePane_PaneNotFound(t *testing.T) {
	m, _ := newTestMutatorWithCapturingPane("content")

	_, err := m.CapturePane(9999, false)
	if err == nil {
		t.Fatal("expected error for unknown pane ID, got nil")
	}
}

func TestCapturePane_EmptyContent(t *testing.T) {
	m, cp := newTestMutatorWithCapturingPane("")

	got, err := m.CapturePane(int(cp.id), false)
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	if got != "" {
		t.Errorf("content = %q, want empty string", got)
	}
}
