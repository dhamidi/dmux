package pane

import (
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/layout"
	"github.com/dhamidi/dmux/internal/pty"
)

// PaneID is an opaque identifier for a pane, aliasing [layout.LeafID].
// A PaneID doubles as the key in [layout.Tree] leaf nodes and in
// [session.Window.Panes].
type PaneID = layout.LeafID

// Cell is a single terminal display cell.
type Cell struct {
	Char rune // displayed character; 0 means empty (treated as space)
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

	// Close shuts down the child process and releases all resources.
	// It blocks until the output-copy goroutine has exited.
	Close() error
}

// Config holds the parameters for creating a new Pane.
// All fields are required.
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
}

// pane is the concrete implementation of Pane.
type pane struct {
	id       PaneID
	pty      pty.PTY
	term     Terminal
	keyEnc   KeyEncoder
	mouseEnc MouseEncoder

	// done is closed by copyOutput when it exits.
	done chan struct{}
}

// New creates a Pane from cfg, starts the output-copy goroutine, and
// returns the Pane interface. New does not start any child process —
// the PTY in cfg must already be open.
func New(cfg Config) (Pane, error) {
	p := &pane{
		id:       cfg.ID,
		pty:      cfg.PTY,
		term:     cfg.Term,
		keyEnc:   cfg.Keys,
		mouseEnc: cfg.Mouse,
		done:     make(chan struct{}),
	}
	go p.copyOutput()
	return p, nil
}

// copyOutput reads bytes from the PTY and feeds them into the terminal
// emulator until the PTY returns an error (typically io.EOF on close).
func (p *pane) copyOutput() {
	defer close(p.done)
	buf := make([]byte, 4096)
	for {
		n, err := p.pty.Read(buf)
		if n > 0 {
			p.term.Write(buf[:n]) //nolint:errcheck
		}
		if err != nil {
			return
		}
	}
}

func (p *pane) ID() PaneID { return p.id }

func (p *pane) Title() string {
	title, _ := p.term.Title()
	return title
}

func (p *pane) Write(data []byte) error {
	_, err := p.pty.Write(data)
	return err
}

func (p *pane) SendKey(key keys.Key) error {
	data, err := p.keyEnc.Encode(key)
	if err != nil {
		return err
	}
	_, err = p.pty.Write(data)
	return err
}

func (p *pane) SendMouse(ev keys.MouseEvent) error {
	data, err := p.mouseEnc.Encode(ev)
	if err != nil {
		return err
	}
	_, err = p.pty.Write(data)
	return err
}

func (p *pane) Resize(cols, rows int) error {
	if err := p.pty.Resize(rows, cols); err != nil {
		return err
	}
	return p.term.Resize(cols, rows)
}

func (p *pane) Snapshot() CellGrid {
	return p.term.Snapshot()
}

// Close shuts down the pane: it closes the encoders and terminal
// emulator, closes the PTY (which signals the child process), and
// waits for the output-copy goroutine to exit.
func (p *pane) Close() error {
	p.keyEnc.Close()
	p.mouseEnc.Close()
	p.term.Close()
	err := p.pty.Close()
	<-p.done
	return err
}
