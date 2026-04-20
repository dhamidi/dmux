package modes

import (
	"fmt"
	"strings"

	"github.com/dhamidi/dmux/internal/keys"
)

// CustomizeOptionEntry is one option shown in the customize overlay.
type CustomizeOptionEntry struct {
	// Scope is the option scope, e.g. "server", "session", or "window".
	Scope string
	// Name is the option name.
	Name string
	// Value is the current option value as a string.
	Value string
}

// CustomizeBindingEntry is one key binding shown in the customize overlay.
type CustomizeBindingEntry struct {
	// Table is the key table name, e.g. "root" or "prefix".
	Table string
	// Key is the string representation of the key, e.g. "C-b".
	Key string
	// Command is the bound command text.
	Command string
}

// customizeNode is one node in the customize tree (both groups and leaves).
type customizeNode struct {
	label    string
	depth    int
	isGroup  bool
	expanded bool
	children []*customizeNode

	// leaf-only fields
	isOption  bool
	optScope  string
	optName   string
	optValue  string

	isBinding bool
	bindTable string
	bindKey   string
	bindCmd   string
}

// CustomizeOverlay is a [ClientOverlay] presenting a tree view of options and
// key bindings that the user can navigate and edit interactively.
//
// Navigation:
//   - Up / Down (or k / j): move selection.
//   - Left / Right (or h / l): collapse / expand tree nodes.
//   - Enter: toggle group or enter edit mode for a leaf.
//   - q / Escape: close the overlay.
//   - /: enter filter mode.
//
// Edit mode: a text input appears at the bottom; Enter commits, Escape cancels.
type CustomizeOverlay struct {
	rect Rect

	// flat contains the currently visible rows (rebuilt on expand/collapse).
	flat   []*customizeNode
	roots  []*customizeNode
	cursor int

	// edit mode state
	editing   bool
	editInput []rune
	editNode  *customizeNode

	// filter mode state
	filtering   bool
	filterInput []rune

	// mutation callbacks
	setOption func(scope, name, value string) error
	bindKey   func(table, key, cmd string) error
}

// NewCustomizeOverlay creates a customize overlay.
//
// rect is the bounding rectangle in screen (client) coordinates.
// options and bindings provide the initial tree content.
// setOption is called when the user commits an option value change.
// bindKey is called when the user commits a key binding change.
// Either callback may be nil if that category of mutation is not supported.
func NewCustomizeOverlay(
	rect Rect,
	options []CustomizeOptionEntry,
	bindings []CustomizeBindingEntry,
	setOption func(scope, name, value string) error,
	bindKey func(table, key, cmd string) error,
) *CustomizeOverlay {
	m := &CustomizeOverlay{
		rect:      rect,
		setOption: setOption,
		bindKey:   bindKey,
	}
	m.roots = m.buildTree(options, bindings)
	m.rebuildFlat()
	return m
}

// buildTree constructs the master tree from options and bindings.
func (m *CustomizeOverlay) buildTree(options []CustomizeOptionEntry, bindings []CustomizeBindingEntry) []*customizeNode {
	// ── Options subtree ─────────────────────────────────────────────────────
	optGroup := &customizeNode{
		label:    "Options",
		depth:    0,
		isGroup:  true,
		expanded: true,
	}

	// Group options by scope, preserving canonical order then extras.
	scopeOrder := []string{"server", "session", "window"}
	byScope := make(map[string][]*customizeNode)
	for _, e := range options {
		leaf := &customizeNode{
			label:     fmt.Sprintf("%s = %s", e.Name, e.Value),
			depth:     2,
			isOption:  true,
			optScope:  e.Scope,
			optName:   e.Name,
			optValue:  e.Value,
		}
		byScope[e.Scope] = append(byScope[e.Scope], leaf)
	}
	// Append any scopes that are not in the canonical list.
	knownScopes := map[string]bool{"server": true, "session": true, "window": true}
	for scope := range byScope {
		if !knownScopes[scope] {
			scopeOrder = append(scopeOrder, scope)
		}
	}
	for _, scope := range scopeOrder {
		leaves, ok := byScope[scope]
		if !ok {
			continue
		}
		sg := &customizeNode{
			label:    scope,
			depth:    1,
			isGroup:  true,
			expanded: true,
			children: leaves,
		}
		optGroup.children = append(optGroup.children, sg)
	}

	// ── Key Bindings subtree ─────────────────────────────────────────────────
	bindGroup := &customizeNode{
		label:    "Key Bindings",
		depth:    0,
		isGroup:  true,
		expanded: true,
	}

	// Group bindings by table, preserving insertion order.
	tableOrder := []string{}
	byTable := make(map[string][]*customizeNode)
	for _, b := range bindings {
		leaf := &customizeNode{
			label:     fmt.Sprintf("%s → %s", b.Key, b.Command),
			depth:     2,
			isBinding: true,
			bindTable: b.Table,
			bindKey:   b.Key,
			bindCmd:   b.Command,
		}
		if _, seen := byTable[b.Table]; !seen {
			tableOrder = append(tableOrder, b.Table)
		}
		byTable[b.Table] = append(byTable[b.Table], leaf)
	}
	for _, table := range tableOrder {
		tg := &customizeNode{
			label:    table,
			depth:    1,
			isGroup:  true,
			expanded: true,
			children: byTable[table],
		}
		bindGroup.children = append(bindGroup.children, tg)
	}

	return []*customizeNode{optGroup, bindGroup}
}

// rebuildFlat reconstructs the flat visible-row slice from the master tree.
func (m *CustomizeOverlay) rebuildFlat() {
	m.flat = m.flat[:0]
	for _, root := range m.roots {
		m.appendVisible(root)
	}
	if len(m.flat) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= len(m.flat) {
		m.cursor = len(m.flat) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// appendVisible recursively appends visible nodes to m.flat.
func (m *CustomizeOverlay) appendVisible(n *customizeNode) {
	m.flat = append(m.flat, n)
	if n.isGroup && n.expanded {
		for _, child := range n.children {
			m.appendVisible(child)
		}
	}
}

// ── ClientOverlay interface ──────────────────────────────────────────────────

// Rect returns the bounding rectangle in screen coordinates.
func (m *CustomizeOverlay) Rect() Rect { return m.rect }

// Render fills dst with the overlay's cells in row-major order.
// len(dst) == Rect().Width * Rect().Height is guaranteed by the host.
func (m *CustomizeOverlay) Render(dst []Cell) {
	w := m.rect.Width
	h := m.rect.Height
	if w <= 0 || h <= 0 {
		return
	}

	// Clear background.
	for i := range dst {
		dst[i] = Cell{Char: ' '}
	}

	// Determine how many rows are available for the list.
	listHeight := h
	if m.editing || m.filtering {
		listHeight = h - 1 // reserve last row for input bar
	}

	// Render list rows.
	for row := 0; row < listHeight && row < len(m.flat); row++ {
		node := m.flat[row]
		selected := row == m.cursor
		m.renderRow(dst, row, w, m.nodeLabel(node), selected)
	}

	// Render input bar at last row.
	if m.editing {
		prompt := "Edit: " + string(m.editInput)
		m.renderRow(dst, h-1, w, prompt, false)
	} else if m.filtering {
		m.renderRow(dst, h-1, w, "/"+string(m.filterInput), false)
	}
}

// nodeLabel returns the display string for a node.
func (m *CustomizeOverlay) nodeLabel(n *customizeNode) string {
	indent := strings.Repeat("  ", n.depth)
	if n.isGroup {
		prefix := ">"
		if n.expanded {
			prefix = "v"
		}
		return indent + prefix + " " + n.label
	}
	return indent + n.label
}

// renderRow writes one text row into dst at the given row index.
func (m *CustomizeOverlay) renderRow(dst []Cell, row, w int, text string, selected bool) {
	runes := []rune(text)
	base := row * w
	for col := 0; col < w; col++ {
		var ch rune = ' '
		if col < len(runes) {
			ch = runes[col]
		}
		c := Cell{Char: ch}
		if selected {
			c.Attrs |= AttrReverse
		}
		if base+col < len(dst) {
			dst[base+col] = c
		}
	}
}

// Key handles a keyboard event.
func (m *CustomizeOverlay) Key(k keys.Key) Outcome {
	if m.editing {
		return m.keyEdit(k)
	}
	if m.filtering {
		return m.keyFilter(k)
	}
	return m.keyNavigate(k)
}

// keyNavigate handles keys in normal navigation mode.
func (m *CustomizeOverlay) keyNavigate(k keys.Key) Outcome {
	switch k.Code {
	case keys.CodeUp:
		if m.cursor > 0 {
			m.cursor--
		}
		return Consumed()
	case keys.CodeDown:
		if m.cursor < len(m.flat)-1 {
			m.cursor++
		}
		return Consumed()
	case keys.CodeLeft:
		m.collapseOrUp()
		return Consumed()
	case keys.CodeRight:
		m.expandOrDown()
		return Consumed()
	case keys.CodeEnter:
		return m.activateSelected()
	case keys.CodeEscape:
		return CloseMode()
	default:
		if k.Code > 0 && k.Mod == 0 {
			switch rune(k.Code) {
			case 'q':
				return CloseMode()
			case 'h':
				m.collapseOrUp()
				return Consumed()
			case 'l':
				m.expandOrDown()
				return Consumed()
			case 'j':
				if m.cursor < len(m.flat)-1 {
					m.cursor++
				}
				return Consumed()
			case 'k':
				if m.cursor > 0 {
					m.cursor--
				}
				return Consumed()
			case '/':
				m.filtering = true
				m.filterInput = m.filterInput[:0]
				return Consumed()
			}
		}
	}
	return Consumed()
}

// collapseOrUp collapses an expanded group, or moves the cursor up one row.
func (m *CustomizeOverlay) collapseOrUp() {
	if m.cursor < len(m.flat) {
		n := m.flat[m.cursor]
		if n.isGroup && n.expanded {
			n.expanded = false
			m.rebuildFlat()
			return
		}
	}
	if m.cursor > 0 {
		m.cursor--
	}
}

// expandOrDown expands a collapsed group, or moves the cursor down one row.
func (m *CustomizeOverlay) expandOrDown() {
	if m.cursor < len(m.flat) {
		n := m.flat[m.cursor]
		if n.isGroup && !n.expanded {
			n.expanded = true
			m.rebuildFlat()
			return
		}
	}
	if m.cursor < len(m.flat)-1 {
		m.cursor++
	}
}

// activateSelected toggles a group or enters edit mode for a leaf node.
func (m *CustomizeOverlay) activateSelected() Outcome {
	if m.cursor >= len(m.flat) {
		return Consumed()
	}
	n := m.flat[m.cursor]
	if n.isGroup {
		n.expanded = !n.expanded
		m.rebuildFlat()
		return Consumed()
	}
	// Enter edit mode.
	m.editing = true
	m.editNode = n
	if n.isOption {
		m.editInput = []rune(n.optValue)
	} else if n.isBinding {
		m.editInput = []rune(n.bindCmd)
	} else {
		m.editInput = m.editInput[:0]
	}
	return Consumed()
}

// keyEdit handles keys while in edit mode.
func (m *CustomizeOverlay) keyEdit(k keys.Key) Outcome {
	switch k.Code {
	case keys.CodeEnter:
		m.commitEdit()
		m.editing = false
		return Consumed()
	case keys.CodeEscape:
		m.editing = false
		m.editNode = nil
		m.editInput = nil
		return Consumed()
	case keys.CodeBackspace:
		if len(m.editInput) > 0 {
			m.editInput = m.editInput[:len(m.editInput)-1]
		}
		return Consumed()
	default:
		if k.Code > 0 && k.Mod == 0 {
			m.editInput = append(m.editInput, rune(k.Code))
		}
		return Consumed()
	}
}

// commitEdit applies the pending edit to the node and calls the mutation callback.
func (m *CustomizeOverlay) commitEdit() {
	if m.editNode == nil {
		return
	}
	val := string(m.editInput)
	if m.editNode.isOption && m.setOption != nil {
		_ = m.setOption(m.editNode.optScope, m.editNode.optName, val)
		m.editNode.optValue = val
		m.editNode.label = fmt.Sprintf("%s = %s", m.editNode.optName, val)
	} else if m.editNode.isBinding && m.bindKey != nil {
		_ = m.bindKey(m.editNode.bindTable, m.editNode.bindKey, val)
		m.editNode.bindCmd = val
		m.editNode.label = fmt.Sprintf("%s → %s", m.editNode.bindKey, val)
	}
	m.editNode = nil
	m.editInput = nil
}

// keyFilter handles keys while in filter mode.
func (m *CustomizeOverlay) keyFilter(k keys.Key) Outcome {
	switch k.Code {
	case keys.CodeEnter, keys.CodeEscape:
		m.filtering = false
		return Consumed()
	case keys.CodeBackspace:
		if len(m.filterInput) > 0 {
			m.filterInput = m.filterInput[:len(m.filterInput)-1]
		}
		return Consumed()
	default:
		if k.Code > 0 && k.Mod == 0 {
			m.filterInput = append(m.filterInput, rune(k.Code))
		}
		return Consumed()
	}
}

// Mouse handles a mouse event.
//
// Clicks inside the overlay update the cursor position and activate the row
// on left-click. Events outside the overlay are passed through.
func (m *CustomizeOverlay) Mouse(ev keys.MouseEvent) Outcome {
	col := ev.Col - m.rect.X
	row := ev.Row - m.rect.Y
	if col < 0 || col >= m.rect.Width || row < 0 || row >= m.rect.Height {
		return Passthrough()
	}
	if row < len(m.flat) {
		m.cursor = row
		if ev.Action == keys.MousePress && ev.Button == keys.MouseLeft {
			return m.activateSelected()
		}
	}
	return Consumed()
}

// CaptureFocus returns true so keyboard events are routed to this overlay.
func (m *CustomizeOverlay) CaptureFocus() bool { return true }

// Close is a no-op; the overlay holds no external resources.
func (m *CustomizeOverlay) Close() {}

// ── Accessors for tests ──────────────────────────────────────────────────────

// Cursor returns the index of the currently selected row.
func (m *CustomizeOverlay) Cursor() int { return m.cursor }

// Editing returns true when the overlay is in edit mode.
func (m *CustomizeOverlay) Editing() bool { return m.editing }

// Filtering returns true when the overlay is in filter mode.
func (m *CustomizeOverlay) Filtering() bool { return m.filtering }

// FlatLen returns the number of currently visible rows.
func (m *CustomizeOverlay) FlatLen() int { return len(m.flat) }
