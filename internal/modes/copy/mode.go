package copy

import (
	"strings"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	"github.com/dhamidi/dmux/internal/style"
)

// Line is a row of display cells from the scrollback buffer.
// Each element corresponds to one terminal column.
type Line []modes.Cell

// Scrollback provides read-only access to a pane's scrollback buffer.
// The concrete implementation wraps a pane's Snapshot; tests may pass
// a stub implementation.
type Scrollback interface {
	// Lines returns all buffered rows, oldest first.
	Lines() []Line
	// Width returns the terminal width in columns.
	Width() int
	// Height returns the terminal height in rows (visible viewport).
	Height() int
}

// CopyCommand is enqueued by [Mode.Command]("copy-selection"). The host
// is responsible for transmitting Text to the clipboard or client
// (for example via OSC 52).
type CopyCommand struct {
	Text string
}

// pos is a (row, col) coordinate in the scrollback buffer.
type pos struct {
	row, col int
}

// searchState tracks the most recent search query and direction.
type searchState struct {
	query   string
	forward bool
}

// matchRange is the location of one query occurrence in the scrollback buffer.
// Row is the line index; StartCol and EndCol are column indices (cell positions).
type matchRange struct {
	row, startCol, endCol int
}

// CopyStyles holds the style strings for copy-mode highlights.
// Each field is a tmux-compatible style string (e.g. "fg=red,bg=black").
// An empty string means no extra styling is applied.
type CopyStyles struct {
	// ModeStyle is applied to the selection range and cursor cell.
	// When empty the default AttrReverse highlight is used.
	ModeStyle string
	// MatchStyle is applied to non-current search match cells.
	MatchStyle string
	// CurrentMatchStyle is applied to the current (active) search match cells.
	CurrentMatchStyle string
	// MarkStyle is applied to all cells on the marked line.
	MarkStyle string
}

// Mode implements [modes.PaneMode] for copy-mode.
//
// It maintains cursor position, an optional selection anchor, search
// state, and optional match/mark highlighting. All mutations go through
// [Mode.Command]; [Mode.Key] maps raw key events to Command calls.
type Mode struct {
	sb         Scrollback
	curRow     int // cursor row in Lines() (0 = top)
	curCol     int // cursor column (0 = left)
	viewOffset int // first row of Lines() shown in the viewport
	selAnchor  *pos
	search     searchState

	styles        CopyStyles
	searchMatches []matchRange // match positions in the current viewport
	currentMatch  int         // index into searchMatches for the active match
	markRow       int         // row of the mark line; -1 when no mark is set
}

// New creates a new Mode backed by sb.
// The cursor starts on the last (most recent) line of the scrollback.
func New(sb Scrollback) *Mode {
	m := &Mode{sb: sb, markRow: -1}
	if lines := sb.Lines(); len(lines) > 0 {
		m.curRow = len(lines) - 1
		h := sb.Height()
		if h > 0 && m.curRow >= h {
			m.viewOffset = m.curRow - h + 1
		}
	}
	return m
}

// CursorRow returns the cursor's row index into Lines().
func (m *Mode) CursorRow() int { return m.curRow }

// CursorCol returns the cursor's column index.
func (m *Mode) CursorCol() int { return m.curCol }

// ViewOffset returns the index of the first visible row of Lines().
func (m *Mode) ViewOffset() int { return m.viewOffset }

// SelectionAnchor returns the anchor (row, col) when a selection is
// active, or (-1, -1) when there is none.
func (m *Mode) SelectionAnchor() (row, col int) {
	if m.selAnchor == nil {
		return -1, -1
	}
	return m.selAnchor.row, m.selAnchor.col
}

// inSelection reports whether the cell at (lineIdx, col) in the scrollback
// buffer falls within the normalised selection range [start, end] (inclusive,
// multi-line).
func inSelection(lineIdx, col int, start, end pos) bool {
	if lineIdx < start.row || lineIdx > end.row {
		return false
	}
	if lineIdx == start.row && col < start.col {
		return false
	}
	if lineIdx == end.row && col > end.col {
		return false
	}
	return true
}

// Render draws the visible portion of the scrollback onto dst.
// The viewport is adjusted so that the cursor is always on-screen.
// Highlights are applied in priority order (lowest to highest):
//  1. Mark-line style (MarkStyle) on the marked row.
//  2. Search match style (MatchStyle / CurrentMatchStyle) on match cells.
//  3. Selection and cursor (AttrReverse XOR, tmux-compatible).
func (m *Mode) Render(dst modes.Canvas) {
	size := dst.Size()
	lines := m.sb.Lines()

	// Keep cursor in view.
	if m.curRow < m.viewOffset {
		m.viewOffset = m.curRow
	} else if size.Rows > 0 && m.curRow >= m.viewOffset+size.Rows {
		m.viewOffset = m.curRow - size.Rows + 1
	}
	if m.viewOffset < 0 {
		m.viewOffset = 0
	}

	// Refresh search match positions for the current viewport.
	m.populateMatches(size.Rows)

	hasSelection := m.selAnchor != nil
	var selStart, selEnd pos
	if hasSelection {
		selStart, selEnd = m.selectionRange()
	}

	for row := 0; row < size.Rows; row++ {
		lineIdx := m.viewOffset + row
		if lineIdx >= len(lines) {
			break
		}
		line := lines[lineIdx]
		for col := 0; col < size.Cols; col++ {
			var c modes.Cell
			if col < len(line) {
				c = line[col]
			}
			if c.Char == 0 {
				c.Char = ' '
			}
			// Apply mark-line style (lowest priority highlight).
			if m.markRow >= 0 && lineIdx == m.markRow {
				c = applyCopyStyle(c, m.styles.MarkStyle)
			}
			// Apply search match highlight (before selection/cursor so
			// that selection and cursor always take visual priority).
			for idx, mr := range m.searchMatches {
				if lineIdx == mr.row && col >= mr.startCol && col < mr.endCol {
					if idx == m.currentMatch {
						c = applyCopyStyle(c, m.styles.CurrentMatchStyle)
					} else {
						c = applyCopyStyle(c, m.styles.MatchStyle)
					}
					break
				}
			}
			// Apply selection highlight.
			if hasSelection && inSelection(lineIdx, col, selStart, selEnd) {
				c.Attrs ^= modes.AttrReverse
			}
			// Apply cursor highlight (on top of selection).
			// When the cursor is inside the selection the two ^= operations
			// cancel out, leaving the cursor cell un-reversed — the expected
			// tmux-compatible behaviour.
			if row == m.curRow-m.viewOffset && col == m.curCol {
				c.Attrs ^= modes.AttrReverse
			}
			dst.Set(col, row, c)
		}
	}
}

// Key handles a raw keyboard event by mapping it to a [Mode.Command] call.
// Unrecognised keys return [modes.Consumed] without any state change.
func (m *Mode) Key(k keys.Key) modes.Outcome {
	switch k.Code {
	case keys.CodeUp, keys.KeyCode('k'):
		return m.Command("cursor-up")
	case keys.CodeDown, keys.KeyCode('j'):
		return m.Command("cursor-down")
	case keys.CodeLeft, keys.KeyCode('h'):
		return m.Command("cursor-left")
	case keys.CodeRight, keys.KeyCode('l'):
		return m.Command("cursor-right")
	case keys.CodeHome, keys.KeyCode('0'):
		return m.Command("start-of-line")
	case keys.CodeEnd, keys.KeyCode('$'):
		return m.Command("end-of-line")
	case keys.CodePageUp:
		return m.Command("page-up")
	case keys.CodePageDown:
		return m.Command("page-down")
	case keys.KeyCode('g'):
		return m.Command("history-top")
	case keys.KeyCode('G'):
		return m.Command("history-bottom")
	case keys.KeyCode('v'):
		return m.Command("begin-selection")
	case keys.KeyCode('y'):
		return m.Command("copy-selection")
	case keys.KeyCode('n'):
		return m.Command("search-again")
	case keys.KeyCode('N'):
		return m.Command("search-reverse")
	case keys.CodeEscape, keys.KeyCode('q'):
		return modes.CloseMode()
	}
	return modes.Consumed()
}

// Mouse is a no-op; copy-mode does not currently handle mouse events.
func (m *Mode) Mouse(_ keys.MouseEvent) modes.Outcome {
	return modes.Consumed()
}

// Close is a no-op; copy-mode holds no resources beyond the Scrollback.
func (m *Mode) Close() {}

// Command executes a named copy-mode command and returns the outcome.
// This is the entry point for `send -X <name>` dispatch from the
// command/builtin package.
//
// Recognised names:
//
//	cursor-up, cursor-down, cursor-left, cursor-right
//	start-of-line, end-of-line
//	page-up, page-down
//	history-top, history-bottom
//	begin-selection, clear-selection, copy-selection
//	search-again, search-reverse
//	cancel
func (m *Mode) Command(name string) modes.Outcome {
	lines := m.sb.Lines()
	switch name {
	case "cursor-up":
		if m.curRow > 0 {
			m.curRow--
			m.clampCol()
			m.scrollToCursor()
		}
	case "cursor-down":
		if m.curRow < len(lines)-1 {
			m.curRow++
			m.clampCol()
			m.scrollToCursor()
		}
	case "cursor-left":
		if m.curCol > 0 {
			m.curCol--
		}
	case "cursor-right":
		if w := m.lineWidth(m.curRow); m.curCol < w-1 {
			m.curCol++
		}
	case "start-of-line":
		m.curCol = 0
	case "end-of-line":
		if w := m.lineWidth(m.curRow); w > 0 {
			m.curCol = w - 1
		}
	case "page-up":
		h := m.sb.Height()
		m.curRow -= h
		if m.curRow < 0 {
			m.curRow = 0
		}
		m.clampCol()
		m.scrollToCursor()
	case "page-down":
		h := m.sb.Height()
		m.curRow += h
		if n := len(lines); n > 0 && m.curRow >= n {
			m.curRow = n - 1
		}
		m.clampCol()
		m.scrollToCursor()
	case "history-top":
		m.curRow = 0
		m.curCol = 0
		m.scrollToCursor()
	case "history-bottom":
		if n := len(lines); n > 0 {
			m.curRow = n - 1
		}
		m.clampCol()
		m.scrollToCursor()
	case "begin-selection":
		m.selAnchor = &pos{row: m.curRow, col: m.curCol}
	case "clear-selection":
		m.selAnchor = nil
	case "copy-selection":
		text := m.selectionText()
		m.selAnchor = nil
		return modes.Command(CopyCommand{Text: text})
	case "search-again":
		m.doSearch(m.search.query, m.search.forward)
	case "search-reverse":
		m.doSearch(m.search.query, !m.search.forward)
	case "set-mark":
		m.markRow = m.curRow
	case "clear-mark":
		m.markRow = -1
	case "cancel":
		return modes.CloseMode()
	}
	return modes.Consumed()
}

// SetSearch sets the search query and direction, then jumps to the
// first match starting from the line after (or before) the cursor.
func (m *Mode) SetSearch(query string, forward bool) {
	m.search.query = query
	m.search.forward = forward
	m.doSearch(query, forward)
}

// ---- private helpers -------------------------------------------------------

func (m *Mode) clampCol() {
	w := m.lineWidth(m.curRow)
	if w == 0 {
		m.curCol = 0
	} else if m.curCol >= w {
		m.curCol = w - 1
	}
}

func (m *Mode) lineWidth(row int) int {
	lines := m.sb.Lines()
	if row < 0 || row >= len(lines) {
		return 0
	}
	return len(lines[row])
}

func (m *Mode) scrollToCursor() {
	h := m.sb.Height()
	if h <= 0 {
		return
	}
	if m.curRow < m.viewOffset {
		m.viewOffset = m.curRow
	} else if m.curRow >= m.viewOffset+h {
		m.viewOffset = m.curRow - h + 1
	}
}

// selectionRange returns the normalised (start, end) positions of the
// active selection. Both are zero-valued when no selection is active.
func (m *Mode) selectionRange() (start, end pos) {
	if m.selAnchor == nil {
		return pos{}, pos{}
	}
	start = *m.selAnchor
	end = pos{row: m.curRow, col: m.curCol}
	if start.row > end.row || (start.row == end.row && start.col > end.col) {
		start, end = end, start
	}
	return start, end
}

// selectionText returns the selected text as a string, with newlines
// between lines. Returns "" when no selection is active.
func (m *Mode) selectionText() string {
	if m.selAnchor == nil {
		return ""
	}
	lines := m.sb.Lines()
	start, end := m.selectionRange()

	var sb strings.Builder
	for r := start.row; r <= end.row && r < len(lines); r++ {
		line := lines[r]
		startCol, endCol := 0, len(line)-1
		if r == start.row {
			startCol = start.col
		}
		if r == end.row {
			endCol = end.col
		}
		if r > start.row {
			sb.WriteByte('\n')
		}
		for c := startCol; c <= endCol && c < len(line); c++ {
			ch := line[c].Char
			if ch == 0 {
				ch = ' '
			}
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}

// doSearch finds the next line containing query in the given direction,
// starting from the line after (or before) the cursor. Wraps around.
func (m *Mode) doSearch(query string, forward bool) {
	if query == "" {
		return
	}
	lines := m.sb.Lines()
	n := len(lines)
	if n == 0 {
		return
	}
	if forward {
		for i := 1; i <= n; i++ {
			r := (m.curRow + i) % n
			if lineContains(lines[r], query) {
				m.curRow = r
				m.scrollToCursor()
				return
			}
		}
	} else {
		for i := 1; i <= n; i++ {
			r := (m.curRow - i + n) % n
			if lineContains(lines[r], query) {
				m.curRow = r
				m.scrollToCursor()
				return
			}
		}
	}
}

// SetStyles configures the highlight style strings used during rendering.
func (m *Mode) SetStyles(s CopyStyles) {
	m.styles = s
}

// MarkRow returns the index of the marked line, or -1 when no mark is set.
func (m *Mode) MarkRow() int { return m.markRow }

// populateMatches scans the current viewport for all occurrences of the
// search query and stores them in m.searchMatches. It also sets m.currentMatch
// to the index of the first match on m.curRow (the match the cursor is on).
// viewHeight is the number of visible rows (dst canvas height).
func (m *Mode) populateMatches(viewHeight int) {
	m.searchMatches = m.searchMatches[:0]
	query := m.search.query
	if query == "" {
		m.currentMatch = 0
		return
	}
	qRunes := []rune(query)
	qLen := len(qRunes)
	if qLen == 0 {
		m.currentMatch = 0
		return
	}
	lines := m.sb.Lines()
	m.currentMatch = -1

	for r := m.viewOffset; r < m.viewOffset+viewHeight && r < len(lines); r++ {
		line := lines[r]
		// Build a rune slice for the line so that column indices are correct.
		lineRunes := make([]rune, len(line))
		for i, c := range line {
			ch := c.Char
			if ch == 0 {
				ch = ' '
			}
			lineRunes[i] = ch
		}
		// Scan for all occurrences of qRunes.
		for start := 0; start+qLen <= len(lineRunes); start++ {
			if runesHavePrefix(lineRunes[start:], qRunes) {
				idx := len(m.searchMatches)
				m.searchMatches = append(m.searchMatches, matchRange{
					row:      r,
					startCol: start,
					endCol:   start + qLen,
				})
				// Mark the first match on the cursor row as the current match.
				if r == m.curRow && m.currentMatch < 0 {
					m.currentMatch = idx
				}
			}
		}
	}
	if m.currentMatch < 0 {
		m.currentMatch = 0
	}
}

// runesHavePrefix reports whether text starts with the rune slice prefix.
func runesHavePrefix(text, prefix []rune) bool {
	if len(text) < len(prefix) {
		return false
	}
	for i, r := range prefix {
		if text[i] != r {
			return false
		}
	}
	return true
}

// applyCopyStyle overlays the attributes from styleStr onto cell c.
// The style string uses tmux-compatible syntax (e.g. "fg=red,bg=black,bold").
// Returns c unchanged when styleStr is empty.
func applyCopyStyle(c modes.Cell, styleStr string) modes.Cell {
	if styleStr == "" {
		return c
	}
	st := style.Parse(styleStr)
	if st.HasFg {
		c.Fg = modes.Color(st.Fg)
		c.FgR, c.FgG, c.FgB = st.FgR, st.FgG, st.FgB
	}
	if st.HasBg {
		c.Bg = modes.Color(st.Bg)
		c.BgR, c.BgG, c.BgB = st.BgR, st.BgG, st.BgB
	}
	// Map style package attribute bits to modes attribute bits.
	// The two packages use different bit positions for the same flags.
	if st.Attrs&style.AttrBold != 0 {
		c.Attrs |= modes.AttrBold
	}
	if st.Attrs&style.AttrUnderscore != 0 {
		c.Attrs |= modes.AttrUnderline
	}
	if st.Attrs&style.AttrBlink != 0 {
		c.Attrs |= modes.AttrBlink
	}
	if st.Attrs&style.AttrReverse != 0 {
		c.Attrs |= modes.AttrReverse
	}
	if st.Attrs&style.AttrDim != 0 {
		c.Attrs |= modes.AttrDim
	}
	return c
}

// lineContains reports whether the text of line contains query.
func lineContains(line Line, query string) bool {
	var sb strings.Builder
	sb.Grow(len(line))
	for _, c := range line {
		ch := c.Char
		if ch == 0 {
			ch = ' '
		}
		sb.WriteRune(ch)
	}
	return strings.Contains(sb.String(), query)
}
