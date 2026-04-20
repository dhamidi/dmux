package server

import (
	"strings"
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/session"
)

// recordingPane is a session.Pane implementation that records SendKey and Write calls.
type recordingPane struct {
	id       session.PaneID
	sentKeys []keys.Key
	written  []byte
}

func (r *recordingPane) Title() string                                { return "recording" }
func (r *recordingPane) Write(data []byte) error                     { r.written = append(r.written, data...); return nil }
func (r *recordingPane) SendKey(key keys.Key) error                  { r.sentKeys = append(r.sentKeys, key); return nil }
func (r *recordingPane) Resize(cols, rows int) error                 { return nil }
func (r *recordingPane) Snapshot() pane.CellGrid                     { return pane.CellGrid{} }
func (r *recordingPane) CaptureContent(history bool) ([]byte, error) { return nil, nil }
func (r *recordingPane) Respawn(shell string) error                  { return nil }
func (r *recordingPane) Close() error                                { return nil }

// newTestMutatorWithRecordingPane creates a serverMutator with a session
// containing a window backed by a recordingPane.
func newTestMutatorWithRecordingPane() (*serverMutator, *recordingPane) {
	state := session.NewServer()
	rp := &recordingPane{}
	m := &serverMutator{
		state:    state,
		shutdown: func() {},
		newPane: func(cfg pane.Config) (session.Pane, error) {
			rp.id = cfg.ID
			return rp, nil
		},
	}
	sv, _ := m.NewSession("s1")
	_, _ = m.NewWindow(sv.ID, "w1")
	return m, rp
}

func TestSendKeys(t *testing.T) {
	m, rp := newTestMutatorWithRecordingPane()
	paneID := int(rp.id)

	if err := m.SendKeys(paneID, []string{"a", "Enter"}); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
	if got := len(rp.sentKeys); got != 2 {
		t.Fatalf("sentKeys length = %d, want 2", got)
	}
}

func TestSendKeys_PaneNotFound(t *testing.T) {
	m, _ := newTestMutatorWithRecordingPane()

	if err := m.SendKeys(9999, []string{"a"}); err == nil {
		t.Fatal("expected error for unknown pane ID, got nil")
	}
}

func TestPasteBuffer(t *testing.T) {
	m, rp := newTestMutatorWithRecordingPane()
	paneID := int(rp.id)

	m.state.Buffers.Set("buf1", "hello")

	if err := m.PasteBuffer("buf1", paneID); err != nil {
		t.Fatalf("PasteBuffer: %v", err)
	}
	if got := string(rp.written); got != "hello" {
		t.Errorf("written = %q, want %q", got, "hello")
	}
}

func TestPasteBuffer_BufferNotFound(t *testing.T) {
	m, rp := newTestMutatorWithRecordingPane()
	paneID := int(rp.id)

	if err := m.PasteBuffer("nonexistent", paneID); err == nil {
		t.Fatal("expected error for unknown buffer name, got nil")
	}
}

func TestRunShell_Foreground(t *testing.T) {
	m, _ := newTestMutatorWithRecordingPane()

	out, err := m.RunShell("echo hello", false)
	if err != nil {
		t.Fatalf("RunShell foreground: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("output = %q, want it to contain %q", out, "hello")
	}
}

func TestRunShell_Background(t *testing.T) {
	m, _ := newTestMutatorWithRecordingPane()

	out, err := m.RunShell("sleep 10", true)
	if err != nil {
		t.Fatalf("RunShell background: %v", err)
	}
	if out != "" {
		t.Errorf("output = %q, want empty string", out)
	}
}
