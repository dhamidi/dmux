package server_test

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/layout"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/server"
	"github.com/dhamidi/dmux/internal/session"
)

// diffTestPane is a fake pane with a mutable cell grid for diff testing.
type diffTestPane struct {
	mu   sync.Mutex
	rows int
	cols int
	buf  []pane.Cell
}

func newDiffTestPane(rows, cols int) *diffTestPane {
	cells := make([]pane.Cell, rows*cols)
	for i := range cells {
		cells[i] = pane.Cell{Char: 'A'}
	}
	return &diffTestPane{rows: rows, cols: cols, buf: cells}
}

func (p *diffTestPane) SetCell(row, col int, ch rune) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.buf[row*p.cols+col].Char = ch
}

func (p *diffTestPane) Snapshot() pane.CellGrid {
	p.mu.Lock()
	defer p.mu.Unlock()
	cells := make([]pane.Cell, len(p.buf))
	copy(cells, p.buf)
	return pane.CellGrid{Rows: p.rows, Cols: p.cols, Cells: cells}
}

func (p *diffTestPane) Title() string                               { return "" }
func (p *diffTestPane) Resize(cols, rows int) error                 { return nil }
func (p *diffTestPane) Close() error                                { return nil }
func (p *diffTestPane) CaptureContent(history bool) ([]byte, error) { return nil, nil }
func (p *diffTestPane) Respawn(shell string) error                  { return nil }
func (p *diffTestPane) SendKey(key keys.Key) error                  { return nil }
func (p *diffTestPane) Write(data []byte) error                     { return nil }
func (p *diffTestPane) ShellPID() int                               { return 0 }
func (p *diffTestPane) LastOutputAt() time.Time                     { return time.Time{} }
func (p *diffTestPane) ConsumeBell() bool                           { return false }
func (p *diffTestPane) ClearHistory()                               {}
func (p *diffTestPane) ClearScreen() error                          { return nil }

// readRenderFrame reads MsgStdout messages from conn until it finds one that
// looks like a render frame (starts with ESC or contains cursor-home). It
// skips messages that look like plain command output.
func readRenderFrame(t *testing.T, conn interface {
	SetDeadline(time.Time) error
	Read([]byte) (int, error)
	Write([]byte) (int, error)
}) []byte {
	t.Helper()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	defer conn.SetDeadline(time.Time{}) //nolint:errcheck
	for {
		msgType, payload, err := proto.ReadMsg(conn)
		if err != nil {
			t.Fatalf("read msg: %v", err)
		}
		if msgType != proto.MsgStdout {
			continue
		}
		var sm proto.StdoutMsg
		if err := sm.Decode(payload); err != nil {
			t.Fatalf("decode StdoutMsg: %v", err)
		}
		// A render frame starts with an ANSI escape sequence (full repaint:
		// "\x1b[H\x1b[2J", or diff: "\x1b[?25l").
		if len(sm.Data) > 0 && sm.Data[0] == '\x1b' {
			return sm.Data
		}
	}
}

// TestRenderLoopDiff verifies that a second render of the same session with one
// changed cell produces a smaller output than the first full repaint.
func TestRenderLoopDiff(t *testing.T) {
	const rows, cols = 24, 80

	fp := newDiffTestPane(rows, cols)

	state := session.NewServer()
	sess := session.NewSession(session.SessionID("$1"), "diff-sess", state.Options)
	paneID := session.PaneID(1)
	win := session.NewWindow(session.WindowID("@1"), "main", sess.Options)
	win.AddPane(paneID, fp)
	win.Layout = layout.New(cols, rows, paneID)
	sess.AddWindow(win)
	state.AddSession(sess)

	pl := newPipeListener()
	sigs := make(chan os.Signal, 1)

	done := startServer(server.Config{
		Listener: pl,
		Log:      io.Discard,
		Signals:  sigs,
		Now:      fixedClock(time.Time{}),
		State:    state,
	})

	clientConn := pl.dial()
	defer clientConn.Close()

	sendHandshake(t, clientConn)
	time.Sleep(10 * time.Millisecond)

	// Attach the client to the session; this triggers a full repaint.
	cm := proto.CommandMsg{Argv: []string{"attach-session", "-t", "diff-sess"}}
	if err := proto.WriteMsg(clientConn, proto.MsgCommand, cm.Encode()); err != nil {
		t.Fatalf("write attach-session: %v", err)
	}

	// Read the first render frame (full repaint).
	firstFrame := readRenderFrame(t, clientConn)
	if !bytes.HasPrefix(firstFrame, []byte("\x1b[H\x1b[2J")) {
		t.Fatalf("expected full-repaint prefix in first frame, got: %q", firstFrame[:min(len(firstFrame), 20)])
	}

	// Change exactly one cell, then trigger a redraw via a no-output MsgStdin.
	fp.SetCell(0, 0, 'Z')

	// Send an unbound keystroke to trigger markDirty without command output.
	stdinMsg := proto.StdinMsg{Data: []byte("x")}
	if err := proto.WriteMsg(clientConn, proto.MsgStdin, stdinMsg.Encode()); err != nil {
		t.Fatalf("write MsgStdin: %v", err)
	}

	// Read the second render frame (diff).
	secondFrame := readRenderFrame(t, clientConn)

	if len(secondFrame) >= len(firstFrame) {
		t.Fatalf("expected diff frame (%d bytes) to be smaller than full repaint (%d bytes)",
			len(secondFrame), len(firstFrame))
	}

	// The diff must NOT contain a full-repaint erase sequence.
	if bytes.Contains(secondFrame, []byte("\x1b[H\x1b[2J")) {
		t.Fatal("second frame should be a diff, not a full repaint")
	}

	sigs <- fakeSignal("SIGTERM")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within timeout after SIGTERM")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
