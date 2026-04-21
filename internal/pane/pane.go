package pane

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/layout"
	"github.com/dhamidi/dmux/internal/pty"
	"github.com/dhamidi/dmux/internal/render"
)

// PaneID is an opaque identifier for a pane, aliasing [layout.LeafID].
// A PaneID doubles as the key in [layout.Tree] leaf nodes and in
// [session.Window.Panes].
type PaneID = layout.LeafID

// Color is an alias for [render.Color], representing an 8-bit terminal color
// index or one of the sentinel values [render.ColorDefault] and [render.ColorRGB].
type Color = render.Color

// Re-export the sentinel Color constants so callers of this package need not
// import render directly.
const (
	ColorDefault = render.ColorDefault
	ColorIndexed = render.ColorIndexed
	ColorRGB     = render.ColorRGB
)

// Cell is a single terminal display cell.
type Cell struct {
	Char  rune  // displayed character; 0 means empty (treated as space)
	Fg    Color // foreground color; ColorDefault means inherit
	Bg    Color // background color; ColorDefault means inherit
	Attrs uint16 // bitmask of SGR attribute flags (see render.Attr* constants)
	// FgR, FgG, FgB are meaningful only when Fg == ColorRGB.
	FgR, FgG, FgB uint8
	// BgR, BgG, BgB are meaningful only when Bg == ColorRGB.
	BgR, BgG, BgB uint8
}

// CellGrid is an immutable snapshot of the visible terminal area.
// Cells are stored in row-major order: Cells[row*Cols+col] is the
// cell at (row, col), with the origin at the top-left corner.
type CellGrid struct {
	Rows  int
	Cols  int
	Cells []Cell
}

// Terminal is the interface a Pane requires of its VT terminal emulator.
// The concrete implementation is a thin adapter over *libghostty.Terminal;
// tests may pass a [FakeTerminal].
//
// All methods must be safe for concurrent calls from a single writer
// goroutine and the caller of [Pane] methods.
type Terminal interface {
	// Write feeds raw PTY output (escape sequences, UTF-8 text, control
	// codes) into the terminal parser.
	Write(p []byte) (int, error)

	// Resize updates the terminal's visible grid to cols columns and
	// rows rows.
	Resize(cols, rows int) error

	// Title returns the current window title set by the child process
	// via OSC 0 or OSC 2.
	Title() (string, error)

	// Snapshot returns an immutable snapshot of the current viewport.
	Snapshot() CellGrid

	// Close releases all resources held by the terminal emulator.
	Close()
}

// KeyEncoder encodes key events into escape sequences suitable for
// writing to a PTY. The concrete implementation wraps the
// go-libghostty key encoder; tests may pass a [FakeKeyEncoder].
type KeyEncoder interface {
	// Encode returns the escape-sequence bytes for key, or an error
	// if the key cannot be represented.
	Encode(key keys.Key) ([]byte, error)

	// Close releases resources held by the encoder.
	Close()
}

// MouseEncoder encodes mouse events into escape sequences suitable for
// writing to a PTY. The concrete implementation wraps the
// go-libghostty mouse encoder; tests may pass a [FakeMouseEncoder].
type MouseEncoder interface {
	// Encode returns the escape-sequence bytes for ev, or an error
	// if the event cannot be represented.
	Encode(ev keys.MouseEvent) ([]byte, error)

	// Close releases resources held by the encoder.
	Close()
}

// Pane is the public interface of a running terminal emulator pane.
// Each pane owns one [pty.PTY], one [Terminal], one [KeyEncoder], one
// [MouseEncoder], and a background goroutine that copies PTY output
// into the Terminal.
//
// The concrete type is unexported; callers receive a Pane from [New].
type Pane interface {
	// ID returns the pane's unique identifier within its window.
	ID() PaneID

	// Title returns the current window title (set via OSC 2 or similar).
	Title() string

	// Write sends raw bytes to the child process as keyboard input.
	Write(data []byte) error

	// SendKey encodes key into escape sequences and writes them to
	// the child process.
	SendKey(key keys.Key) error

	// SendMouse encodes ev into escape sequences and writes them to
	// the child process.
	SendMouse(ev keys.MouseEvent) error

	// Resize updates both the PTY window size and the terminal grid
	// dimensions to cols columns and rows rows.
	Resize(cols, rows int) error

	// Snapshot returns an immutable snapshot of the visible terminal state.
	Snapshot() CellGrid

	// CaptureContent returns the visible terminal content as plain text.
	// If history is true, scrollback content is included when available.
	CaptureContent(history bool) ([]byte, error)

	// Respawn kills the current child process and starts a fresh one using
	// the given shell (falls back to $SHELL or /bin/sh if empty).
	// Returns an error if no PTYFactory was configured.
	Respawn(shell string) error

	// Close shuts down the child process and releases all resources.
	// It blocks until the output-copy goroutine has exited.
	Close() error
	// ShellPID returns the PID of the pane's direct child process (the shell).
	// Returns 0 if the process has exited or the PTY is closed.
	ShellPID() int
	// LastOutputAt returns the time of the most recent PTY output written to
	// the terminal. Returns the zero time if no output has been written yet.
	LastOutputAt() time.Time
	// ConsumeBell returns true if a BEL character (\x07) was received since
	// the last call, and resets the flag.
	ConsumeBell() bool

	// ClearHistory discards all lines stored in the pane's scrollback buffer.
	ClearHistory()

	// ClearScreen injects the ANSI erase-display sequence into the pane's
	// pseudo-terminal, causing the visible area to be blanked.
	ClearScreen() error

	// HasPipe reports whether a pipe subprocess is currently attached.
	HasPipe() bool

	// AttachPipe starts shellCmd via "sh -c shellCmd" and routes PTY output
	// to its stdin. Any previously attached pipe is stopped first.
	// Returns an error if the subprocess cannot be started.
	AttachPipe(shellCmd string) error

	// DetachPipe stops any active pipe subprocess attached by [AttachPipe].
	// It is a no-op if no pipe is attached.
	DetachPipe() error
}

// Config holds the parameters for creating a new Pane.
// All fields except PTYFactory are required.
type Config struct {
	// ID is the pane's unique identifier within its window.
	ID PaneID

	// PTY is the pseudo-terminal connected to the child process.
	PTY pty.PTY

	// Term is the VT terminal emulator that parses PTY output.
	Term Terminal

	// Keys encodes key events for the child process.
	Keys KeyEncoder

	// Mouse encodes mouse events for the child process.
	Mouse MouseEncoder

	// PTYFactory is called by Respawn to create a replacement PTY.
	// The arguments are the shell command, current cols, and current rows.
	// If nil, Respawn returns an error.
	PTYFactory func(shell string, cols, rows int) (pty.PTY, error)
}

// pane is the concrete implementation of Pane.
type pane struct {
	id       PaneID
	term     Terminal
	keyEnc   KeyEncoder
	mouseEnc MouseEncoder

	mu           sync.Mutex
	ptyField     pty.PTY
	cols, rows   int
	ptyFactory   func(shell string, cols, rows int) (pty.PTY, error)
	lastOutputAt time.Time // updated each time the PTY reader writes to the terminal
	bellPending  bool      // set when \x07 (BEL) is detected in output

	// Scrollback stores lines that have scrolled off the visible area.
	scrollback [][]Cell

	// pipeCmd is the subprocess started by pipe-pane, if any.
	pipeCmd *exec.Cmd
	// pipeWriter is the stdin pipe of pipeCmd; PTY output is teed to it.
	pipeWriter io.WriteCloser
	// pipeDone is closed when the pipe goroutines have all exited.
	pipeDone chan struct{}

	// done is closed by copyOutput when it exits.
	done chan struct{}
}

// New creates a Pane from cfg, starts the output-copy goroutine, and
// returns the Pane interface. New does not start any child process —
// the PTY in cfg must already be open.
func New(cfg Config) (Pane, error) {
	p := &pane{
		id:         cfg.ID,
		ptyField:   cfg.PTY,
		term:       cfg.Term,
		keyEnc:     cfg.Keys,
		mouseEnc:   cfg.Mouse,
		ptyFactory: cfg.PTYFactory,
		done:       make(chan struct{}),
	}
	go p.copyOutput()
	return p, nil
}

// copyOutput reads bytes from the PTY and feeds them into the terminal
// emulator until the PTY returns an error (typically io.EOF on close).
// If a pipe subprocess is attached, output is also forwarded to its stdin.
func (p *pane) copyOutput() {
	defer close(p.done)
	buf := make([]byte, 4096)
	for {
		p.mu.Lock()
		currentPTY := p.ptyField
		p.mu.Unlock()

		n, err := currentPTY.Read(buf)
		if n > 0 {
			p.term.Write(buf[:n]) //nolint:errcheck
			p.mu.Lock()
			p.lastOutputAt = time.Now()
			if bytes.IndexByte(buf[:n], '\x07') >= 0 {
				p.bellPending = true
			}
			w := p.pipeWriter
			p.mu.Unlock()
			if w != nil {
				w.Write(buf[:n]) //nolint:errcheck // best-effort; pipe errors don't affect terminal
			}
		}
		if err != nil {
			return
		}
	}
}

// HasPipe reports whether a pipe subprocess is currently attached.
func (p *pane) HasPipe() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pipeCmd != nil
}

// AttachPipe starts shellCmd via "sh -c shellCmd" and routes future PTY
// output to its stdin. Any previously attached pipe is stopped first.
func (p *pane) AttachPipe(shellCmd string) error {
	cmd := exec.Command("sh", "-c", shellCmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("attach-pipe: get stdin pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		stdin.Close() //nolint:errcheck
		return fmt.Errorf("attach-pipe: start subprocess: %w", err)
	}

	done := make(chan struct{})
	p.mu.Lock()
	p.pipeCmd = cmd
	p.pipeWriter = stdin
	p.pipeDone = done
	p.mu.Unlock()

	// Reap the subprocess in the background; clean up pipe fields on exit.
	go func() {
		defer close(done)
		_ = cmd.Wait()
		p.mu.Lock()
		if p.pipeCmd == cmd {
			p.pipeCmd = nil
			p.pipeWriter = nil
			p.pipeDone = nil
		}
		p.mu.Unlock()
	}()

	return nil
}

// DetachPipe stops any active pipe subprocess. It is a no-op when no pipe
// is attached. Stdin is closed first so the subprocess can flush before being
// terminated; if the subprocess does not exit within 200 ms it is killed.
func (p *pane) DetachPipe() error {
	p.mu.Lock()
	cmd := p.pipeCmd
	w := p.pipeWriter
	done := p.pipeDone
	if cmd != nil {
		p.pipeCmd = nil
		p.pipeWriter = nil
		p.pipeDone = nil
	}
	p.mu.Unlock()

	if cmd == nil {
		return nil
	}

	// Close stdin so the subprocess sees EOF and can flush buffered output.
	if w != nil {
		w.Close() //nolint:errcheck
	}

	// Give the subprocess a short window to exit cleanly after stdin closes.
	if done != nil {
		select {
		case <-done:
			// Subprocess exited on its own after stdin closed.
		case <-time.After(200 * time.Millisecond):
			// Timeout: force-kill and wait for the reaper goroutine.
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-done
		}
	}
	return nil
}

// LastOutputAt returns the time of the most recent write to the terminal.
func (p *pane) LastOutputAt() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastOutputAt
}

// ConsumeBell returns true if a BEL character was received since the last call,
// and resets the flag.
func (p *pane) ConsumeBell() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	v := p.bellPending
	p.bellPending = false
	return v
}

func (p *pane) ID() PaneID { return p.id }

func (p *pane) Title() string {
	title, _ := p.term.Title()
	return title
}

func (p *pane) Write(data []byte) error {
	p.mu.Lock()
	currentPTY := p.ptyField
	p.mu.Unlock()
	_, err := currentPTY.Write(data)
	return err
}

func (p *pane) SendKey(key keys.Key) error {
	data, err := p.keyEnc.Encode(key)
	if err != nil {
		return err
	}
	p.mu.Lock()
	currentPTY := p.ptyField
	p.mu.Unlock()
	_, err = currentPTY.Write(data)
	return err
}

func (p *pane) SendMouse(ev keys.MouseEvent) error {
	data, err := p.mouseEnc.Encode(ev)
	if err != nil {
		return err
	}
	p.mu.Lock()
	currentPTY := p.ptyField
	p.mu.Unlock()
	_, err = currentPTY.Write(data)
	return err
}

func (p *pane) Resize(cols, rows int) error {
	p.mu.Lock()
	currentPTY := p.ptyField
	p.mu.Unlock()

	if err := currentPTY.Resize(rows, cols); err != nil {
		return err
	}
	if err := p.term.Resize(cols, rows); err != nil {
		return err
	}
	p.mu.Lock()
	p.cols, p.rows = cols, rows
	p.mu.Unlock()
	return nil
}

func (p *pane) Snapshot() CellGrid {
	return p.term.Snapshot()
}

// ShellPID returns the PID of the direct child process running inside the PTY.
// Returns 0 if the PTY is nil or the process has exited.
func (p *pane) ShellPID() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ptyField == nil {
		return 0
	}
	return p.ptyField.Pid()
}

// CaptureContent renders the visible terminal grid as plain text.
// Each row is terminated by a newline. If history is true, the same
// visible content is returned (full scrollback requires Terminal interface
// extensions not yet implemented).
func (p *pane) CaptureContent(_ bool) ([]byte, error) {
	grid := p.term.Snapshot()
	var buf bytes.Buffer
	for row := 0; row < grid.Rows; row++ {
		for col := 0; col < grid.Cols; col++ {
			ch := grid.Cells[row*grid.Cols+col].Char
			if ch == 0 {
				ch = ' '
			}
			buf.WriteRune(ch)
		}
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// Respawn closes the current child process and starts a fresh one. The shell
// argument names the executable to run; if empty, $SHELL is used, falling
// back to /bin/sh. Returns an error if no PTYFactory was configured.
func (p *pane) Respawn(shell string) error {
	p.mu.Lock()
	factory := p.ptyFactory
	cols, rows := p.cols, p.rows
	oldPTY := p.ptyField
	p.mu.Unlock()

	if factory == nil {
		return fmt.Errorf("respawn: no PTY factory configured")
	}

	if shell == "" {
		shell = os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
	}

	// Close the old PTY (signals EOF to copyOutput goroutine) and wait for it.
	oldPTY.Close() //nolint:errcheck
	<-p.done

	// Start a replacement PTY.
	newPTY, err := factory(shell, cols, rows)
	if err != nil {
		return fmt.Errorf("respawn: %w", err)
	}

	p.mu.Lock()
	p.ptyField = newPTY
	p.done = make(chan struct{})
	p.mu.Unlock()

	go p.copyOutput()
	return nil
}

// ClearHistory discards all lines stored in the scrollback buffer.
func (p *pane) ClearHistory() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.scrollback = p.scrollback[:0]
}

// ClearScreen injects the ANSI cursor-home + erase-display sequence into the
// pane's pseudo-terminal, causing the visible area to be blanked.
func (p *pane) ClearScreen() error {
	return p.Write([]byte("\x1b[H\x1b[2J"))
}

// Close shuts down the pane: it stops any attached pipe subprocess, closes
// the encoders and terminal emulator, closes the PTY (which signals the child
// process), and waits for the output-copy goroutine to exit.
func (p *pane) Close() error {
	p.DetachPipe() //nolint:errcheck
	p.keyEnc.Close()
	p.mouseEnc.Close()
	p.term.Close()
	p.mu.Lock()
	currentPTY := p.ptyField
	p.mu.Unlock()
	err := currentPTY.Close()
	<-p.done
	return err
}
