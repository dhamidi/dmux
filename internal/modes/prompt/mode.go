package prompt

import (
	"strings"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
)

// Config holds constructor parameters for a command-prompt overlay.
//
// All behaviour is injected via callbacks; the prompt does not import
// internal/command or read any external state.
type Config struct {
	// Prompt is the label rendered before the editable input field.
	Prompt string
	// Initial is pre-populated text placed in the input buffer at construction.
	// The cursor is positioned at the end of the initial text.
	Initial string
	// Template is a %-escape template applied to the input on confirm.
	// Every occurrence of %% in Template is replaced with the confirmed
	// input string. If Template is empty, the raw input is used.
	Template string
	// History contains initial history entries (oldest first, newest last).
	// The slice is not modified; a private copy is made at construction.
	History []string
	// OnConfirm is called with the final (template-expanded) input when
	// the user presses Enter. May be nil.
	OnConfirm func(input string)
	// OnCancel is called when the user presses Escape. May be nil.
	OnCancel func()
	// Complete returns completion candidates for the given partial input.
	// It is called on the first Tab press; successive Tab presses cycle
	// through the returned slice. A nil Complete disables completion.
	Complete func(partial string) []string
}

// CommandMode implements [modes.ClientOverlay] for command-prompt.
//
// Construct with [NewCommand]; do not copy after first use.
type CommandMode struct {
	cfg    Config
	rect   modes.Rect
	buf    []rune // current line buffer
	cursor int    // cursor index: 0 ≤ cursor ≤ len(buf)

	// history navigation
	history    []string // all entries (oldest first)
	histIdx    int      // -1 = not navigating history
	savedInput string   // input saved before entering history navigation

	// tab completion state
	inCompletion bool
	completions  []string
	compIdx      int    // index of the next candidate to apply
	compSuffix   []rune // buf[cursor:] captured when completion started
}

// NewCommand creates a command-prompt overlay occupying rect.
//
// The caller owns the returned *CommandMode and is responsible for
// storing any history updates after the overlay closes.
func NewCommand(rect modes.Rect, cfg Config) *CommandMode {
	buf := []rune(cfg.Initial)
	return &CommandMode{
		cfg:     cfg,
		rect:    rect,
		buf:     buf,
		cursor:  len(buf),
		history: append([]string(nil), cfg.History...),
		histIdx: -1,
	}
}

// Rect returns the overlay's bounding rectangle in screen coordinates.
func (m *CommandMode) Rect() modes.Rect { return m.rect }

// Render fills dst with the prompt's cells in row-major order.
// len(dst) == Rect().Width * Rect().Height is guaranteed by the host.
func (m *CommandMode) Render(dst []modes.Cell) {
	for i := range dst {
		dst[i] = modes.Cell{Char: ' '}
	}
	col := 0
	for _, r := range m.cfg.Prompt {
		if col >= len(dst) {
			return
		}
		dst[col] = modes.Cell{Char: r}
		col++
	}
	for _, r := range m.buf {
		if col >= len(dst) {
			return
		}
		dst[col] = modes.Cell{Char: r}
		col++
	}
	// Show a cursor indicator when the cursor is at the end of the buffer.
	if m.cursor == len(m.buf) && col < len(dst) {
		dst[col] = modes.Cell{Char: '_'}
	}
}

// Key handles a keyboard event.
//
// Any key other than Tab resets the tab-completion cycle.
func (m *CommandMode) Key(k keys.Key) modes.Outcome {
	if k.Code != keys.CodeTab {
		m.inCompletion = false
	}

	switch k.Code {
	case keys.CodeEnter:
		input := string(m.buf)
		if m.cfg.Template != "" {
			input = applyTemplate(m.cfg.Template, input)
		}
		if m.cfg.OnConfirm != nil {
			m.cfg.OnConfirm(input)
		}
		return modes.CloseMode()

	case keys.CodeEscape:
		if m.cfg.OnCancel != nil {
			m.cfg.OnCancel()
		}
		return modes.CloseMode()

	case keys.CodeBackspace:
		m.deleteBackward()

	case keys.CodeDelete:
		m.deleteForward()

	case keys.CodeLeft:
		if k.Mod&keys.ModCtrl != 0 {
			m.moveWordLeft()
		} else {
			m.moveCursor(-1)
		}

	case keys.CodeRight:
		if k.Mod&keys.ModCtrl != 0 {
			m.moveWordRight()
		} else {
			m.moveCursor(1)
		}

	case keys.CodeHome:
		m.cursor = 0

	case keys.CodeEnd:
		m.cursor = len(m.buf)

	case keys.CodeUp:
		m.historyBack()

	case keys.CodeDown:
		m.historyForward()

	case keys.CodeTab:
		m.tabComplete()

	default:
		if k.Mod == keys.ModCtrl {
			switch rune(k.Code) {
			case 'a':
				m.cursor = 0
			case 'e':
				m.cursor = len(m.buf)
			case 'w':
				m.deleteWordBackward()
			case 'k':
				m.killToEnd()
			}
		} else if k.Code > 0 && k.Mod == 0 {
			m.insertRune(rune(k.Code))
		}
	}

	return modes.Consumed()
}

// Mouse passes through mouse events; the prompt does not handle the mouse.
func (m *CommandMode) Mouse(_ keys.MouseEvent) modes.Outcome { return modes.Passthrough() }

// CaptureFocus returns true so keyboard events are delivered to the prompt.
func (m *CommandMode) CaptureFocus() bool { return true }

// Close is a no-op; the prompt holds no external resources.
func (m *CommandMode) Close() {}

// Input returns the current contents of the line buffer as a string.
// Useful for inspection in tests and by the host when closing the overlay.
func (m *CommandMode) Input() string { return string(m.buf) }

// CursorPos returns the current cursor position (0-based index into the buffer).
func (m *CommandMode) CursorPos() int { return m.cursor }

// ---- line-editing helpers --------------------------------------------------

func (m *CommandMode) insertRune(r rune) {
	m.buf = append(m.buf, 0)
	copy(m.buf[m.cursor+1:], m.buf[m.cursor:])
	m.buf[m.cursor] = r
	m.cursor++
}

func (m *CommandMode) deleteBackward() {
	if m.cursor == 0 {
		return
	}
	m.buf = append(m.buf[:m.cursor-1], m.buf[m.cursor:]...)
	m.cursor--
}

func (m *CommandMode) deleteForward() {
	if m.cursor >= len(m.buf) {
		return
	}
	m.buf = append(m.buf[:m.cursor], m.buf[m.cursor+1:]...)
}

func (m *CommandMode) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.buf) {
		m.cursor = len(m.buf)
	}
}

func (m *CommandMode) moveWordLeft() {
	for m.cursor > 0 && m.buf[m.cursor-1] == ' ' {
		m.cursor--
	}
	for m.cursor > 0 && m.buf[m.cursor-1] != ' ' {
		m.cursor--
	}
}

func (m *CommandMode) moveWordRight() {
	n := len(m.buf)
	for m.cursor < n && m.buf[m.cursor] == ' ' {
		m.cursor++
	}
	for m.cursor < n && m.buf[m.cursor] != ' ' {
		m.cursor++
	}
}

func (m *CommandMode) deleteWordBackward() {
	end := m.cursor
	for m.cursor > 0 && m.buf[m.cursor-1] == ' ' {
		m.cursor--
	}
	for m.cursor > 0 && m.buf[m.cursor-1] != ' ' {
		m.cursor--
	}
	m.buf = append(m.buf[:m.cursor], m.buf[end:]...)
}

func (m *CommandMode) killToEnd() {
	m.buf = m.buf[:m.cursor]
}

// ---- history helpers -------------------------------------------------------

func (m *CommandMode) historyBack() {
	if len(m.history) == 0 {
		return
	}
	if m.histIdx == -1 {
		m.savedInput = string(m.buf)
		m.histIdx = len(m.history) - 1
	} else if m.histIdx > 0 {
		m.histIdx--
	}
	m.setInput(m.history[m.histIdx])
}

func (m *CommandMode) historyForward() {
	if m.histIdx == -1 {
		return
	}
	if m.histIdx < len(m.history)-1 {
		m.histIdx++
		m.setInput(m.history[m.histIdx])
	} else {
		m.histIdx = -1
		m.setInput(m.savedInput)
	}
}

func (m *CommandMode) setInput(s string) {
	m.buf = []rune(s)
	m.cursor = len(m.buf)
}

// ---- tab-completion helpers ------------------------------------------------

func (m *CommandMode) tabComplete() {
	if m.cfg.Complete == nil {
		return
	}
	if !m.inCompletion {
		// Capture the suffix after the cursor before we start cycling.
		m.compSuffix = append([]rune(nil), m.buf[m.cursor:]...)
		m.completions = m.cfg.Complete(string(m.buf[:m.cursor]))
		m.compIdx = 0
		m.inCompletion = len(m.completions) > 0
		if !m.inCompletion {
			return
		}
	}
	comp := []rune(m.completions[m.compIdx])
	m.compIdx = (m.compIdx + 1) % len(m.completions)
	m.buf = append(comp, m.compSuffix...)
	m.cursor = len(comp)
}

// ---- template helper -------------------------------------------------------

// applyTemplate replaces every occurrence of %% in tmpl with input.
func applyTemplate(tmpl, input string) string {
	return strings.ReplaceAll(tmpl, "%%", input)
}

// ============================================================================
// ConfirmMode
// ============================================================================

// ConfirmMode implements [modes.ClientOverlay] for confirm-before dialogs.
//
// Accepts y / Y / Enter as confirmation; any other key triggers cancellation.
// Construct with [NewConfirm]; do not copy after first use.
type ConfirmMode struct {
	prompt string
	onYes  func()
	onNo   func()
	rect   modes.Rect
}

// NewConfirm creates a confirm-before overlay occupying rect.
//
// onYes is called when the user presses y, Y, or Enter.
// onNo is called for any other key press. Either callback may be nil.
func NewConfirm(rect modes.Rect, prompt string, onYes, onNo func()) *ConfirmMode {
	return &ConfirmMode{
		prompt: prompt,
		onYes:  onYes,
		onNo:   onNo,
		rect:   rect,
	}
}

// Rect returns the overlay's bounding rectangle in screen coordinates.
func (m *ConfirmMode) Rect() modes.Rect { return m.rect }

// Render fills dst with the prompt label, padded with spaces.
func (m *ConfirmMode) Render(dst []modes.Cell) {
	for i := range dst {
		dst[i] = modes.Cell{Char: ' '}
	}
	col := 0
	for _, r := range m.prompt {
		if col >= len(dst) {
			return
		}
		dst[col] = modes.Cell{Char: r}
		col++
	}
}

// Key handles keyboard events.
//
// y / Y / Enter call onYes; any other key calls onNo. The overlay always
// closes after the first key press.
func (m *ConfirmMode) Key(k keys.Key) modes.Outcome {
	switch k.Code {
	case keys.CodeEnter, keys.KeyCode('y'), keys.KeyCode('Y'):
		if m.onYes != nil {
			m.onYes()
		}
	default:
		if m.onNo != nil {
			m.onNo()
		}
	}
	return modes.CloseMode()
}

// Mouse passes through mouse events.
func (m *ConfirmMode) Mouse(_ keys.MouseEvent) modes.Outcome { return modes.Passthrough() }

// CaptureFocus returns true so keyboard events are delivered to the dialog.
func (m *ConfirmMode) CaptureFocus() bool { return true }

// Close is a no-op; the confirm dialog holds no external resources.
func (m *ConfirmMode) Close() {}
