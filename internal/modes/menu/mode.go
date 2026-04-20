package menu

import (
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
)

// MenuItem is a single entry in the menu.
//
// When the item is activated (via Enter, mnemonic key, or mouse click),
// OnSelect is called and the menu closes. The menu never dispatches
// commands itself — all behaviour is driven by the provided callback.
type MenuItem struct {
	Label     string  // displayed text
	Mnemonic  rune    // optional single-key shortcut; 0 means none
	Separator bool    // render as a horizontal rule; not selectable
	Enabled   bool    // if false the item is shown but cannot be activated
	OnSelect  func()  // called on activation; may be nil
}

// Mode implements [modes.ClientOverlay] for display-menu.
//
// It is constructed via [New] and must not be copied after first use.
type Mode struct {
	items    []MenuItem
	selected int        // index of highlighted item; -1 = no selectable items
	rect     modes.Rect // bounding rectangle in screen (client) coordinates
}

// itemWidth returns the display width of an item row in cells.
// Width accounts for the two-character prefix ("  " or "> ") plus the label runes.
func itemWidth(label string) int {
	return len([]rune(label)) + 2
}

// computeRect derives the menu's bounding rectangle from the anchor position
// and the item list. The anchor's X/Y become the top-left corner.
func computeRect(anchor modes.Rect, items []MenuItem) modes.Rect {
	width := 4 // minimum: prefix (2) + at least 2 chars
	for _, item := range items {
		if item.Separator {
			continue
		}
		if w := itemWidth(item.Label); w > width {
			width = w
		}
	}
	return modes.Rect{
		X:      anchor.X,
		Y:      anchor.Y,
		Width:  width,
		Height: len(items),
	}
}

// firstSelectable returns the index of the first enabled, non-separator item,
// or -1 when no such item exists.
func firstSelectable(items []MenuItem) int {
	for i, item := range items {
		if !item.Separator && item.Enabled {
			return i
		}
	}
	return -1
}

// New creates a menu overlay.
//
// anchor supplies the top-left position in screen (client) coordinates.
// The menu self-sizes to the widest label. items is the list of entries
// presented to the user; the slice is not modified.
func New(anchor modes.Rect, items []MenuItem) *Mode {
	m := &Mode{
		items:    items,
		selected: firstSelectable(items),
		rect:     computeRect(anchor, items),
	}
	return m
}

// Rect returns the overlay's bounding rectangle in screen coordinates.
func (m *Mode) Rect() modes.Rect { return m.rect }

// Render fills dst with the menu's cells in row-major order.
// len(dst) == Rect().Width * Rect().Height is guaranteed by the host.
func (m *Mode) Render(dst []modes.Cell) {
	w := m.rect.Width
	for row, item := range m.items {
		if row >= m.rect.Height {
			break
		}
		base := row * w
		if item.Separator {
			for col := 0; col < w; col++ {
				if base+col < len(dst) {
					dst[base+col] = modes.Cell{Char: '─'}
				}
			}
			continue
		}
		// Two-character prefix: marker + space.
		prefix := ' '
		if row == m.selected {
			prefix = '>'
		}
		if base < len(dst) {
			dst[base] = modes.Cell{Char: prefix}
		}
		if base+1 < len(dst) {
			dst[base+1] = modes.Cell{Char: ' '}
		}
		// Label runes.
		runes := []rune(item.Label)
		for col := 2; col < w; col++ {
			var ch rune = ' '
			if col-2 < len(runes) {
				ch = runes[col-2]
			}
			if base+col < len(dst) {
				dst[base+col] = modes.Cell{Char: ch}
			}
		}
	}
}

// Key handles a keyboard event.
//
//   - Up / Down move the selection to the previous / next enabled item.
//   - Enter activates the currently selected item.
//   - Escape closes the menu without activating anything.
//   - Any rune matching an item's Mnemonic activates that item directly.
//   - All other keys are consumed without effect.
func (m *Mode) Key(k keys.Key) modes.Outcome {
	switch k.Code {
	case keys.CodeUp:
		m.movePrev()
		return modes.Consumed()
	case keys.CodeDown:
		m.moveNext()
		return modes.Consumed()
	case keys.CodeEnter:
		return m.activate(m.selected)
	case keys.CodeEscape:
		return modes.CloseMode()
	default:
		if k.Code > 0 {
			ch := rune(k.Code)
			for i, item := range m.items {
				if !item.Separator && item.Enabled && item.Mnemonic == ch {
					return m.activate(i)
				}
			}
		}
	}
	return modes.Consumed()
}

// Mouse handles mouse events.
//
// When the pointer is inside the menu rectangle, motion events update
// the hover highlight. A left-button press activates the item under
// the pointer. Events outside the menu are passed through.
func (m *Mode) Mouse(ev keys.MouseEvent) modes.Outcome {
	col := ev.Col - m.rect.X
	row := ev.Row - m.rect.Y
	if col < 0 || col >= m.rect.Width || row < 0 || row >= m.rect.Height {
		return modes.Passthrough()
	}
	if row < len(m.items) {
		item := m.items[row]
		if !item.Separator && item.Enabled {
			m.selected = row
			if ev.Action == keys.MousePress && ev.Button == keys.MouseLeft {
				return m.activate(row)
			}
		}
	}
	return modes.Consumed()
}

// CaptureFocus returns true so keyboard events are delivered to the menu
// rather than the focused pane underneath.
func (m *Mode) CaptureFocus() bool { return true }

// Close is a no-op; the menu holds no external resources.
func (m *Mode) Close() {}

// Selected returns the index of the currently highlighted item, or -1.
func (m *Mode) Selected() int { return m.selected }

// ---- private helpers -------------------------------------------------------

// activate calls OnSelect (if set) on items[idx] and returns CloseMode.
// Returns Consumed when idx is out of range or the item is not selectable.
func (m *Mode) activate(idx int) modes.Outcome {
	if idx < 0 || idx >= len(m.items) {
		return modes.CloseMode()
	}
	item := m.items[idx]
	if item.Separator || !item.Enabled {
		return modes.Consumed()
	}
	if item.OnSelect != nil {
		item.OnSelect()
	}
	return modes.CloseMode()
}

// moveNext advances the selection to the next enabled, non-separator item,
// wrapping around. Does nothing when no selectable item exists.
func (m *Mode) moveNext() {
	n := len(m.items)
	if n == 0 {
		return
	}
	start := m.selected
	if start < 0 {
		start = n - 1
	}
	i := (start + 1) % n
	for i != start {
		if !m.items[i].Separator && m.items[i].Enabled {
			m.selected = i
			return
		}
		i = (i + 1) % n
	}
}

// movePrev moves the selection to the previous enabled, non-separator item,
// wrapping around. Does nothing when no selectable item exists.
func (m *Mode) movePrev() {
	n := len(m.items)
	if n == 0 {
		return
	}
	start := m.selected
	if start < 0 {
		start = 0
	}
	i := (start - 1 + n) % n
	for i != start {
		if !m.items[i].Separator && m.items[i].Enabled {
			m.selected = i
			return
		}
		i = (i - 1 + n) % n
	}
}
