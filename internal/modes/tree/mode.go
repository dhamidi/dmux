package tree

import (
	"strings"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	"github.com/dhamidi/dmux/internal/pane"
)

// NodeKind identifies the type of a TreeNode.
type NodeKind int

const (
	KindSession NodeKind = iota // top-level tmux session
	KindWindow                  // window within a session
	KindPane                    // pane within a window
)

// TreeNode is a plain-data snapshot of one entry in the session/window/pane
// tree. It carries no live references; the caller builds the tree from
// whatever source (session server, config file, test fixture, …) and passes
// the root slice to [New].
type TreeNode struct {
	Kind     NodeKind   // Session, Window, or Pane
	ID       string     // opaque identifier passed to [OnSelect]
	Name     string     // human-readable label shown in the list
	Children []TreeNode // nested nodes (windows under sessions, panes under windows)
}

// PreviewProvider is called when the tree mode needs to render the preview
// pane.  It receives the ID of the currently-highlighted node and returns a
// cell-grid snapshot to display, or nil if no preview is available.
type PreviewProvider func(id string) *pane.CellGrid

// flatNode is an internal flattened entry used for linear navigation.
type flatNode struct {
	node  TreeNode
	depth int // indentation level (0 = session)
}

// Mode implements [modes.PaneMode] for the session/window/pane chooser.
//
// It holds a flattened view of the tree for O(1) cursor movement, an
// optional search query, and references to the [OnSelect] callback and
// [PreviewProvider].  No live session objects are referenced.
type Mode struct {
	flat       []flatNode
	cursor     int
	searching  bool   // true while the user is typing a search query
	search     string // current search query (empty ⇒ show all nodes)
	onSelect   func(id string)
	preview    PreviewProvider
}

// New creates a new Mode backed by the given tree snapshot.
//
//   - nodes is the top-level list of [TreeNode] values (typically one entry
//     per session).
//   - onSelect is called with the selected node's ID when the user presses
//     Enter; it may be nil.
//   - preview may be nil; when non-nil the right half of the pane shows a
//     cell-grid snapshot of the currently-highlighted node.
func New(nodes []TreeNode, onSelect func(id string), preview PreviewProvider) *Mode {
	return &Mode{
		flat:     flattenNodes(nodes, 0),
		onSelect: onSelect,
		preview:  preview,
	}
}

// flattenNodes recursively flattens nodes into a depth-annotated linear list.
func flattenNodes(nodes []TreeNode, depth int) []flatNode {
	var out []flatNode
	for _, n := range nodes {
		out = append(out, flatNode{node: n, depth: depth})
		out = append(out, flattenNodes(n.Children, depth+1)...)
	}
	return out
}

// Cursor returns the current cursor index into the visible node list.
func (m *Mode) Cursor() int { return m.cursor }

// Search returns the current search query string.
func (m *Mode) Search() string { return m.search }

// Searching reports whether search mode is active.
func (m *Mode) Searching() bool { return m.searching }

// SelectedID returns the ID of the currently highlighted node, or "" if the
// list is empty.
func (m *Mode) SelectedID() string {
	visible := m.visibleNodes()
	if len(visible) == 0 || m.cursor >= len(visible) {
		return ""
	}
	return visible[m.cursor].node.ID
}

// SetSearch sets the search query and resets the cursor to the first match.
// Passing an empty string clears the filter.
func (m *Mode) SetSearch(query string) {
	m.search = query
	m.cursor = 0
}

// Render draws the tree list (and an optional preview) onto dst.
//
// Layout: when a [PreviewProvider] is set and the canvas is at least 4 columns
// wide, the left half shows the tree list and the right half shows the
// preview.  Otherwise the full width is used for the list.
func (m *Mode) Render(dst modes.Canvas) {
	size := dst.Size()
	if size.Cols == 0 || size.Rows == 0 {
		return
	}

	listCols := size.Cols
	previewCols := 0
	previewOffset := 0
	if m.preview != nil && size.Cols >= 4 {
		listCols = size.Cols / 2
		previewCols = size.Cols - listCols
		previewOffset = listCols
	}

	visible := m.visibleNodes()
	cur := m.clampedCursor(visible)

	// Keep cursor in the viewport.
	viewOffset := 0
	if size.Rows > 0 && cur >= size.Rows {
		viewOffset = cur - size.Rows + 1
	}

	// Render list column.
	for row := 0; row < size.Rows; row++ {
		idx := viewOffset + row
		if idx >= len(visible) {
			break
		}
		fn := visible[idx]
		label := m.nodeLabel(fn)
		col := 0
		for _, ch := range label {
			if col >= listCols {
				break
			}
			dst.Set(col, row, modes.Cell{Char: ch})
			col++
		}
		for ; col < listCols; col++ {
			dst.Set(col, row, modes.Cell{Char: ' '})
		}
	}

	// Render preview column.
	if previewCols > 0 && m.preview != nil {
		var nodeID string
		if len(visible) > 0 && cur < len(visible) {
			nodeID = visible[cur].node.ID
		}
		if nodeID != "" {
			if grid := m.preview(nodeID); grid != nil {
				for row := 0; row < size.Rows && row < grid.Rows; row++ {
					for col := 0; col < previewCols && col < grid.Cols; col++ {
						cell := grid.Cells[row*grid.Cols+col]
						ch := cell.Char
						if ch == 0 {
							ch = ' '
						}
						dst.Set(previewOffset+col, row, modes.Cell{Char: ch})
					}
				}
			}
		}
	}
}

// Key handles a keyboard event.
//
// Normal mode key bindings:
//
//	Up / k          move cursor up
//	Down / j        move cursor down
//	/               enter search mode
//	Enter           select current node and close mode
//	Escape / q      close mode without selecting
//
// Search mode key bindings (active after pressing /):
//
//	printable rune  append to search query
//	Backspace       remove last character from query
//	Enter           confirm query and return to normal mode
//	Escape          clear query and return to normal mode
func (m *Mode) Key(k keys.Key) modes.Outcome {
	if m.searching {
		return m.handleSearchKey(k)
	}
	return m.handleNormalKey(k)
}

// Mouse is a no-op; tree mode does not currently handle mouse events.
func (m *Mode) Mouse(_ keys.MouseEvent) modes.Outcome {
	return modes.Consumed()
}

// Close is a no-op; tree mode holds no resources beyond the snapshot slices.
func (m *Mode) Close() {}

// ---- private helpers -------------------------------------------------------

func (m *Mode) handleNormalKey(k keys.Key) modes.Outcome {
	switch k.Code {
	case keys.CodeUp, keys.KeyCode('k'):
		m.moveCursor(-1)
	case keys.CodeDown, keys.KeyCode('j'):
		m.moveCursor(1)
	case keys.KeyCode('/'):
		m.searching = true
		m.search = ""
		m.cursor = 0
	case keys.CodeEnter:
		return m.doSelect()
	case keys.CodeEscape, keys.KeyCode('q'):
		return modes.CloseMode()
	}
	return modes.Consumed()
}

func (m *Mode) handleSearchKey(k keys.Key) modes.Outcome {
	switch k.Code {
	case keys.CodeEscape:
		m.searching = false
		m.search = ""
		m.cursor = 0
	case keys.CodeEnter:
		m.searching = false
	case keys.CodeBackspace:
		if len(m.search) > 0 {
			runes := []rune(m.search)
			m.search = string(runes[:len(runes)-1])
			m.cursor = 0
		}
	default:
		if k.Code > 0x1f && k.Code < 0x7f {
			m.search += string(rune(k.Code))
			m.cursor = 0
		}
	}
	return modes.Consumed()
}

func (m *Mode) moveCursor(delta int) {
	visible := m.visibleNodes()
	n := len(visible)
	if n == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	} else if m.cursor >= n {
		m.cursor = n - 1
	}
}

func (m *Mode) doSelect() modes.Outcome {
	visible := m.visibleNodes()
	if len(visible) == 0 {
		return modes.CloseMode()
	}
	cur := m.clampedCursor(visible)
	id := visible[cur].node.ID
	if m.onSelect != nil {
		m.onSelect(id)
	}
	return modes.CloseMode()
}

func (m *Mode) visibleNodes() []flatNode {
	if m.search == "" {
		return m.flat
	}
	q := strings.ToLower(m.search)
	var out []flatNode
	for _, fn := range m.flat {
		if strings.Contains(strings.ToLower(fn.node.Name), q) {
			out = append(out, fn)
		}
	}
	return out
}

func (m *Mode) clampedCursor(visible []flatNode) int {
	if len(visible) == 0 {
		return 0
	}
	if m.cursor >= len(visible) {
		return len(visible) - 1
	}
	if m.cursor < 0 {
		return 0
	}
	return m.cursor
}

// nodeLabel returns the indented display label for fn.
func (m *Mode) nodeLabel(fn flatNode) string {
	return strings.Repeat("  ", fn.depth) + fn.node.Name
}
