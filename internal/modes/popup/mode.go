package popup

import (
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	"github.com/dhamidi/dmux/internal/pane"
)

// Pane is the subset of [pane.Pane] that the popup uses.
// Declaring it locally documents which methods the popup actually calls,
// and lets tests inject a fake without implementing the full pane.Pane.
type Pane interface {
	// Write sends raw bytes to the child process.
	Write(data []byte) error
	// SendKey encodes key and writes it to the child process.
	SendKey(key keys.Key) error
	// SendMouse encodes ev and writes it to the child process.
	SendMouse(ev keys.MouseEvent) error
	// Resize updates the PTY and terminal dimensions.
	Resize(cols, rows int) error
	// Snapshot returns an immutable snapshot of the visible terminal state.
	Snapshot() pane.CellGrid
	// Close shuts down the child process and releases all resources.
	Close() error
}

// PaneFactory creates a Pane sized to rows×cols running command.
// It is called once in [New]; tests may inject a fake factory.
type PaneFactory func(rows, cols int, command string) (Pane, error)

// Mode implements [modes.ClientOverlay] for display-popup.
//
// The outer rectangle includes a one-cell border on all sides. The inner
// rectangle (rect shrunk by 1 on each side) is where the pane's output
// is rendered. Keys and mouse events inside the popup are forwarded to
// the pane; Escape closes the popup.
//
// Mode must not be copied after first use.
type Mode struct {
	rect   modes.Rect
	pane   Pane
	closed bool
}

// New creates a popup overlay at rect, running command inside a pane sized
// to the inner area (rect minus the 1-cell border on each side).
//
// autoClose is reserved for callers that want the popup to close when the
// command exits (the -E flag). It is stored but not acted upon by Mode
// itself — the host is responsible for detecting command exit.
//
// factory is called once to create the underlying pane; pass a real PTY
// factory for production or a [FakePaneFactory] in tests.
func New(rect modes.Rect, command string, _ bool, factory PaneFactory) (*Mode, error) {
	rows := rect.Height - 2
	cols := rect.Width - 2
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	p, err := factory(rows, cols, command)
	if err != nil {
		return nil, err
	}
	return &Mode{rect: rect, pane: p}, nil
}

// Rect returns the overlay's bounding rectangle in screen (client) coordinates.
func (m *Mode) Rect() modes.Rect { return m.rect }

// Render fills dst with the popup's cells in row-major order.
// It draws a single-line box border and copies the pane snapshot
// into the interior. len(dst) == Rect().Width * Rect().Height is
// guaranteed by the host.
func (m *Mode) Render(dst []modes.Cell) {
	w := m.rect.Width
	h := m.rect.Height
	if w < 2 || h < 2 || len(dst) < w*h {
		return
	}

	set := func(col, row int, ch rune) {
		if idx := row*w + col; idx >= 0 && idx < len(dst) {
			dst[idx] = modes.Cell{Char: ch}
		}
	}

	// Corners.
	set(0, 0, '┌')
	set(w-1, 0, '┐')
	set(0, h-1, '└')
	set(w-1, h-1, '┘')

	// Top and bottom edges.
	for col := 1; col < w-1; col++ {
		set(col, 0, '─')
		set(col, h-1, '─')
	}

	// Left and right edges.
	for row := 1; row < h-1; row++ {
		set(0, row, '│')
		set(w-1, row, '│')
	}

	// Copy pane snapshot into the interior.
	snap := m.pane.Snapshot()
	innerW := w - 2
	innerH := h - 2
	for row := 0; row < innerH; row++ {
		for col := 0; col < innerW; col++ {
			ch := rune(' ')
			if row < snap.Rows && col < snap.Cols {
				if c := snap.Cells[row*snap.Cols+col].Char; c != 0 {
					ch = c
				}
			}
			set(col+1, row+1, ch)
		}
	}
}

// Key handles a keyboard event.
//
// Escape closes the popup. All other keys are forwarded to the pane.
func (m *Mode) Key(k keys.Key) modes.Outcome {
	if k.Code == keys.CodeEscape {
		return modes.CloseMode()
	}
	m.pane.SendKey(k) //nolint:errcheck
	return modes.Consumed()
}

// Mouse handles a mouse event.
//
// Events inside the popup rectangle are forwarded to the pane and
// consumed. Events outside are passed through to the layer below.
func (m *Mode) Mouse(ev keys.MouseEvent) modes.Outcome {
	col := ev.Col - m.rect.X
	row := ev.Row - m.rect.Y
	if col < 0 || col >= m.rect.Width || row < 0 || row >= m.rect.Height {
		return modes.Passthrough()
	}
	m.pane.SendMouse(ev) //nolint:errcheck
	return modes.Consumed()
}

// CaptureFocus returns true so keyboard events are delivered to the popup
// rather than the focused pane underneath.
func (m *Mode) CaptureFocus() bool { return true }

// Close kills the underlying pane. It is safe to call Close more than once.
func (m *Mode) Close() {
	if !m.closed {
		m.closed = true
		m.pane.Close() //nolint:errcheck
	}
}
