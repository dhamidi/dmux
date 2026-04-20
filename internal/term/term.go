package term

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
)

// SizeFunc queries the current terminal dimensions.
// It returns the number of rows and columns, or an error.
type SizeFunc func() (rows, cols int, err error)

// RawModeFunc enters raw (unbuffered, no-echo) mode on the terminal.
// It returns a restore function that returns the terminal to its previous
// state, plus any error encountered while switching modes.
type RawModeFunc func() (restore func() error, err error)

// Config holds the injected I/O dependencies for a [Term].
// Out and Size are required; In and RawMode may be nil.
type Config struct {
	// In is the source of raw terminal input bytes (typically os.Stdin or a
	// TTY opened for reading). May be nil if the caller does not read input.
	In io.Reader

	// Out is the destination for escape sequences (typically os.Stdout or a
	// TTY opened for writing). Must not be nil.
	Out io.Writer

	// Size queries the current terminal dimensions. Must not be nil.
	Size SizeFunc

	// RawMode enters raw mode on the terminal and returns a restore function.
	// If nil, Open does not attempt to change the terminal mode.
	RawMode RawModeFunc
}

// colorMode distinguishes how a Color value is interpreted.
type colorMode uint8

const (
	colorDefault colorMode = iota // terminal default; zero value
	colorPalette                  // 256-color palette index
	colorRGB                      // 24-bit RGB
)

// Color represents a terminal cell color.
// The zero value is the terminal's default foreground or background color.
type Color struct {
	mode    colorMode
	index   uint8
	r, g, b uint8
}

// DefaultColor returns the terminal's default color.
// It is identical to the zero value Color{}.
func DefaultColor() Color { return Color{} }

// PaletteColor returns a color from the 256-color xterm palette (index 0–255).
func PaletteColor(i uint8) Color { return Color{mode: colorPalette, index: i} }

// RGBColor returns a 24-bit RGB color.
func RGBColor(r, g, b uint8) Color { return Color{mode: colorRGB, r: r, g: g, b: b} }

// Attr holds text display attributes for a terminal cell.
type Attr uint16

const (
	AttrBold            Attr = 1 << iota // bold / increased intensity (SGR 1)
	AttrUnderline                        // single underline (SGR 4)
	AttrBlink                            // blinking text (SGR 5)
	AttrReverse                          // swap foreground and background (SGR 7)
	AttrDim                              // dim / decreased intensity (SGR 2)
	AttrItalics                          // italic (SGR 3)
	AttrOverline                         // overline (SGR 53)
	AttrStrikethrough                    // strikethrough (SGR 9)
	AttrDoubleUnderline                  // double underline (SGR 21)
	AttrCurlyUnderline                   // curly underline (SGR 4:3)
)

// Cell is a single terminal cell: a rune plus styling.
// The zero value is a space with default colors and no attributes.
type Cell struct {
	Rune rune
	FG   Color
	BG   Color
	Attr Attr
}

// Term manages the client's real terminal: raw mode, size queries, and
// cell-grid output via diffed escape sequences.
// All I/O is performed through the injected [Config].
type Term struct {
	mu      sync.Mutex
	cfg     Config
	rows    int
	cols    int
	cells   []Cell // current frame (rows×cols, row-major)
	flushed []Cell // last flushed frame; Rune=-1 means "not yet written"
	restore func() error
}

// Open creates a new Term using cfg.
// It queries the initial size via cfg.Size and optionally enters raw mode
// via cfg.RawMode (skipped when cfg.RawMode is nil).
func Open(cfg Config) (*Term, error) {
	if cfg.Out == nil {
		return nil, fmt.Errorf("term.Open: Config.Out must not be nil")
	}
	if cfg.Size == nil {
		return nil, fmt.Errorf("term.Open: Config.Size must not be nil")
	}
	rows, cols, err := cfg.Size()
	if err != nil {
		return nil, fmt.Errorf("term.Open: query size: %w", err)
	}
	n := rows * cols
	flushed := make([]Cell, n)
	for i := range flushed {
		flushed[i].Rune = -1 // sentinel: force full redraw on first Flush
	}
	t := &Term{
		cfg:     cfg,
		rows:    rows,
		cols:    cols,
		cells:   make([]Cell, n),
		flushed: flushed,
	}
	if cfg.RawMode != nil {
		restore, err := cfg.RawMode()
		if err != nil {
			return nil, fmt.Errorf("term.Open: enter raw mode: %w", err)
		}
		t.restore = restore
	}
	return t, nil
}

// Close restores the terminal to its state before [Open] was called.
// It is safe to call Close multiple times; subsequent calls are no-ops.
func (t *Term) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.restore != nil {
		err := t.restore()
		t.restore = nil
		return err
	}
	return nil
}

// Size re-queries the terminal dimensions via the injected [SizeFunc].
// If the dimensions have changed, the internal cell grid is resized and
// a subsequent [Term.Flush] will repaint the entire screen.
func (t *Term) Size() (rows, cols int, err error) {
	rows, cols, err = t.cfg.Size()
	if err != nil {
		return 0, 0, err
	}
	t.mu.Lock()
	if rows != t.rows || cols != t.cols {
		t.resize(rows, cols)
	}
	t.mu.Unlock()
	return rows, cols, nil
}

// resize reallocates the cell grids to the new dimensions, preserving
// the overlapping region. Must be called with t.mu held.
func (t *Term) resize(rows, cols int) {
	n := rows * cols
	cells := make([]Cell, n)
	flushed := make([]Cell, n)
	copyRows := t.rows
	if rows < copyRows {
		copyRows = rows
	}
	copyCols := t.cols
	if cols < copyCols {
		copyCols = cols
	}
	for r := 0; r < copyRows; r++ {
		for c := 0; c < copyCols; c++ {
			cells[r*cols+c] = t.cells[r*t.cols+c]
		}
	}
	for i := range flushed {
		flushed[i].Rune = -1 // force full redraw after resize
	}
	t.rows, t.cols, t.cells, t.flushed = rows, cols, cells, flushed
}

// SetCell sets the cell at the given (row, col) position.
// Both row and col are zero-based. Out-of-bounds calls are silently ignored.
func (t *Term) SetCell(row, col int, cell Cell) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if row < 0 || row >= t.rows || col < 0 || col >= t.cols {
		return
	}
	t.cells[row*t.cols+col] = cell
}

// Clear fills the entire cell grid with empty cells (space, default colors).
func (t *Term) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := range t.cells {
		t.cells[i] = Cell{}
	}
}

// Flush computes the diff between the current cell grid and the last flushed
// frame and emits the minimum set of VT/ANSI escape sequences to cfg.Out
// needed to synchronise the real terminal. The cursor is hidden during the
// update and restored afterwards.
func (t *Term) Flush() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	var buf bytes.Buffer
	buf.WriteString("\x1b[?25l") // hide cursor during update

	lastRow, lastCol := -1, -1
	for row := 0; row < t.rows; row++ {
		for col := 0; col < t.cols; col++ {
			idx := row*t.cols + col
			cell := t.cells[idx]
			if cell == t.flushed[idx] {
				lastCol = -1 // break adjacency: next changed cell needs explicit move
				continue
			}
			if row != lastRow || col != lastCol+1 {
				fmt.Fprintf(&buf, "\x1b[%d;%dH", row+1, col+1)
			}
			writeCell(&buf, cell)
			t.flushed[idx] = cell
			lastRow, lastCol = row, col
		}
	}

	buf.WriteString("\x1b[0m")   // reset SGR attributes
	buf.WriteString("\x1b[?25h") // show cursor

	_, err := t.cfg.Out.Write(buf.Bytes())
	return err
}

// writeCell appends the SGR sequence and rune for cell to buf.
func writeCell(buf *bytes.Buffer, cell Cell) {
	params := make([]string, 0, 6)
	params = append(params, "0") // reset all attributes first

	if cell.Attr&AttrBold != 0 {
		params = append(params, "1")
	}
	if cell.Attr&AttrDim != 0 {
		params = append(params, "2")
	}
	if cell.Attr&AttrItalics != 0 {
		params = append(params, "3")
	}
	if cell.Attr&AttrUnderline != 0 {
		params = append(params, "4")
	}
	if cell.Attr&AttrBlink != 0 {
		params = append(params, "5")
	}
	if cell.Attr&AttrReverse != 0 {
		params = append(params, "7")
	}
	if cell.Attr&AttrStrikethrough != 0 {
		params = append(params, "9")
	}
	if cell.Attr&AttrDoubleUnderline != 0 {
		params = append(params, "21")
	}
	if cell.Attr&AttrOverline != 0 {
		params = append(params, "53")
	}

	switch cell.FG.mode {
	case colorPalette:
		params = append(params, fmt.Sprintf("38;5;%d", cell.FG.index))
	case colorRGB:
		params = append(params, fmt.Sprintf("38;2;%d;%d;%d", cell.FG.r, cell.FG.g, cell.FG.b))
	}

	switch cell.BG.mode {
	case colorPalette:
		params = append(params, fmt.Sprintf("48;5;%d", cell.BG.index))
	case colorRGB:
		params = append(params, fmt.Sprintf("48;2;%d;%d;%d", cell.BG.r, cell.BG.g, cell.BG.b))
	}

	fmt.Fprintf(buf, "\x1b[%sm", strings.Join(params, ";"))

	if cell.Attr&AttrCurlyUnderline != 0 {
		buf.WriteString("\x1b[4:3m")
	}

	r := cell.Rune
	if r == 0 {
		r = ' '
	}
	buf.WriteRune(r)
}
