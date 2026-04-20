package prompt_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	"github.com/dhamidi/dmux/internal/modes/prompt"
)

// Compile-time interface assertions.
var _ modes.ClientOverlay = (*prompt.CommandMode)(nil)
var _ modes.ClientOverlay = (*prompt.ConfirmMode)(nil)

func defaultRect() modes.Rect {
	return modes.Rect{X: 0, Y: 23, Width: 80, Height: 1}
}

func press(code keys.KeyCode) keys.Key {
	return keys.Key{Code: code}
}

func pressRune(r rune) keys.Key {
	return keys.Key{Code: keys.KeyCode(r)}
}

func pressCtrl(r rune) keys.Key {
	return keys.Key{Code: keys.KeyCode(r), Mod: keys.ModCtrl}
}

// ============================================================================
// CommandMode — character input and basic editing
// ============================================================================

func TestCommandMode_CharInput(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	m.Key(pressRune('h'))
	m.Key(pressRune('i'))
	if got := m.Input(); got != "hi" {
		t.Errorf("Input() = %q, want %q", got, "hi")
	}
	if got := m.CursorPos(); got != 2 {
		t.Errorf("CursorPos() = %d, want 2", got)
	}
}

func TestCommandMode_Backspace(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	m.Key(pressRune('a'))
	m.Key(pressRune('b'))
	m.Key(pressRune('c'))
	m.Key(press(keys.CodeBackspace))
	if got := m.Input(); got != "ab" {
		t.Errorf("Input() = %q, want %q", got, "ab")
	}
	if got := m.CursorPos(); got != 2 {
		t.Errorf("CursorPos() = %d, want 2", got)
	}
}

func TestCommandMode_BackspaceAtStart(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	m.Key(press(keys.CodeBackspace)) // no-op at start
	if got := m.Input(); got != "" {
		t.Errorf("Input() = %q, want %q", got, "")
	}
	if got := m.CursorPos(); got != 0 {
		t.Errorf("CursorPos() = %d, want 0", got)
	}
}

func TestCommandMode_DeleteForward(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	m.Key(pressRune('a'))
	m.Key(pressRune('b'))
	m.Key(pressRune('c'))
	m.Key(press(keys.CodeHome))
	m.Key(press(keys.CodeDelete))
	if got := m.Input(); got != "bc" {
		t.Errorf("Input() = %q, want %q", got, "bc")
	}
	if got := m.CursorPos(); got != 0 {
		t.Errorf("CursorPos() = %d, want 0", got)
	}
}

func TestCommandMode_DeleteAtEnd(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	m.Key(pressRune('a'))
	m.Key(press(keys.CodeDelete)) // no-op at end
	if got := m.Input(); got != "a" {
		t.Errorf("Input() = %q, want %q", got, "a")
	}
}

func TestCommandMode_InsertAtCursor(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	m.Key(pressRune('a'))
	m.Key(pressRune('c'))
	m.Key(press(keys.CodeLeft))
	m.Key(pressRune('b'))
	if got := m.Input(); got != "abc" {
		t.Errorf("Input() = %q, want %q", got, "abc")
	}
	if got := m.CursorPos(); got != 2 {
		t.Errorf("CursorPos() = %d, want 2", got)
	}
}

func TestCommandMode_CursorMovement(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	m.Key(pressRune('a'))
	m.Key(pressRune('b'))
	m.Key(pressRune('c'))

	m.Key(press(keys.CodeLeft))
	if got := m.CursorPos(); got != 2 {
		t.Errorf("after Left, CursorPos() = %d, want 2", got)
	}

	m.Key(press(keys.CodeRight))
	if got := m.CursorPos(); got != 3 {
		t.Errorf("after Right, CursorPos() = %d, want 3", got)
	}

	// Moving left past start clamps to 0.
	m.Key(press(keys.CodeHome))
	m.Key(press(keys.CodeLeft))
	if got := m.CursorPos(); got != 0 {
		t.Errorf("after Left at start, CursorPos() = %d, want 0", got)
	}

	// Moving right past end clamps to len.
	m.Key(press(keys.CodeEnd))
	m.Key(press(keys.CodeRight))
	if got := m.CursorPos(); got != 3 {
		t.Errorf("after Right at end, CursorPos() = %d, want 3", got)
	}
}

func TestCommandMode_HomeEnd(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	m.Key(pressRune('a'))
	m.Key(pressRune('b'))

	m.Key(press(keys.CodeHome))
	if got := m.CursorPos(); got != 0 {
		t.Errorf("after Home, CursorPos() = %d, want 0", got)
	}

	m.Key(press(keys.CodeEnd))
	if got := m.CursorPos(); got != 2 {
		t.Errorf("after End, CursorPos() = %d, want 2", got)
	}
}

func TestCommandMode_CtrlA_CtrlE(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	m.Key(pressRune('x'))
	m.Key(pressRune('y'))

	m.Key(pressCtrl('a'))
	if got := m.CursorPos(); got != 0 {
		t.Errorf("after C-a, CursorPos() = %d, want 0", got)
	}

	m.Key(pressCtrl('e'))
	if got := m.CursorPos(); got != 2 {
		t.Errorf("after C-e, CursorPos() = %d, want 2", got)
	}
}

func TestCommandMode_CtrlK(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	m.Key(pressRune('a'))
	m.Key(pressRune('b'))
	m.Key(pressRune('c'))
	m.Key(press(keys.CodeLeft))
	m.Key(pressCtrl('k'))
	if got := m.Input(); got != "ab" {
		t.Errorf("after C-k, Input() = %q, want %q", got, "ab")
	}
	if got := m.CursorPos(); got != 2 {
		t.Errorf("after C-k, CursorPos() = %d, want 2", got)
	}
}

func TestCommandMode_CtrlW(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	m.Key(pressRune('h'))
	m.Key(pressRune('e'))
	m.Key(pressRune('l'))
	m.Key(pressRune('l'))
	m.Key(pressRune('o'))
	m.Key(pressCtrl('w'))
	if got := m.Input(); got != "" {
		t.Errorf("after C-w, Input() = %q, want %q", got, "")
	}
}

func TestCommandMode_CtrlW_WordBoundary(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	for _, r := range "hello world" {
		m.Key(pressRune(r))
	}
	m.Key(pressCtrl('w'))
	if got := m.Input(); got != "hello " {
		t.Errorf("after C-w, Input() = %q, want %q", got, "hello ")
	}
}

func TestCommandMode_WordMovement(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	for _, r := range "foo bar" {
		m.Key(pressRune(r))
	}
	// cursor at end (7)
	m.Key(keys.Key{Code: keys.CodeLeft, Mod: keys.ModCtrl})
	if got := m.CursorPos(); got != 4 {
		t.Errorf("after C-Left, CursorPos() = %d, want 4", got)
	}
	m.Key(keys.Key{Code: keys.CodeLeft, Mod: keys.ModCtrl})
	if got := m.CursorPos(); got != 0 {
		t.Errorf("after C-Left x2, CursorPos() = %d, want 0", got)
	}
	m.Key(keys.Key{Code: keys.CodeRight, Mod: keys.ModCtrl})
	if got := m.CursorPos(); got != 3 {
		t.Errorf("after C-Right, CursorPos() = %d, want 3", got)
	}
}

// ============================================================================
// CommandMode — history navigation
// ============================================================================

func TestCommandMode_HistoryNavigation(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{
		History: []string{"cmd1", "cmd2", "cmd3"},
	})

	m.Key(press(keys.CodeUp))
	if got := m.Input(); got != "cmd3" {
		t.Errorf("after Up x1, Input() = %q, want %q", got, "cmd3")
	}
	m.Key(press(keys.CodeUp))
	if got := m.Input(); got != "cmd2" {
		t.Errorf("after Up x2, Input() = %q, want %q", got, "cmd2")
	}
	m.Key(press(keys.CodeUp))
	if got := m.Input(); got != "cmd1" {
		t.Errorf("after Up x3, Input() = %q, want %q", got, "cmd1")
	}

	m.Key(press(keys.CodeDown))
	if got := m.Input(); got != "cmd2" {
		t.Errorf("after Down x1, Input() = %q, want %q", got, "cmd2")
	}
	m.Key(press(keys.CodeDown))
	if got := m.Input(); got != "cmd3" {
		t.Errorf("after Down x2, Input() = %q, want %q", got, "cmd3")
	}
	m.Key(press(keys.CodeDown))
	// should restore the original saved input (empty)
	if got := m.Input(); got != "" {
		t.Errorf("after Down past end, Input() = %q, want %q", got, "")
	}
}

func TestCommandMode_HistorySavesCurrentInput(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{
		History: []string{"old"},
	})
	m.Key(pressRune('n'))
	m.Key(pressRune('e'))
	m.Key(pressRune('w'))

	m.Key(press(keys.CodeUp))
	if got := m.Input(); got != "old" {
		t.Errorf("after Up, Input() = %q, want %q", got, "old")
	}

	m.Key(press(keys.CodeDown))
	if got := m.Input(); got != "new" {
		t.Errorf("after Down, Input() = %q, want %q", got, "new")
	}
}

func TestCommandMode_HistoryAtOldestClamped(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{
		History: []string{"only"},
	})
	m.Key(press(keys.CodeUp))
	m.Key(press(keys.CodeUp)) // extra Up should not move past oldest
	if got := m.Input(); got != "only" {
		t.Errorf("after extra Up, Input() = %q, want %q", got, "only")
	}
}

func TestCommandMode_HistoryEmpty(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	m.Key(pressRune('x'))
	m.Key(press(keys.CodeUp)) // no-op when history is empty
	if got := m.Input(); got != "x" {
		t.Errorf("after Up with empty history, Input() = %q, want %q", got, "x")
	}
}

// ============================================================================
// CommandMode — tab completion
// ============================================================================

func TestCommandMode_TabCompletion(t *testing.T) {
	completions := []string{"find-window", "find-pane", "find-session"}
	m := prompt.NewCommand(defaultRect(), prompt.Config{
		Complete: func(partial string) []string {
			if partial == "find" {
				return completions
			}
			return nil
		},
	})
	for _, r := range "find" {
		m.Key(pressRune(r))
	}

	m.Key(press(keys.CodeTab))
	if got := m.Input(); got != "find-window" {
		t.Errorf("Tab x1, Input() = %q, want %q", got, "find-window")
	}

	m.Key(press(keys.CodeTab))
	if got := m.Input(); got != "find-pane" {
		t.Errorf("Tab x2, Input() = %q, want %q", got, "find-pane")
	}

	m.Key(press(keys.CodeTab))
	if got := m.Input(); got != "find-session" {
		t.Errorf("Tab x3, Input() = %q, want %q", got, "find-session")
	}

	m.Key(press(keys.CodeTab))
	if got := m.Input(); got != "find-window" {
		t.Errorf("Tab x4 (wrap), Input() = %q, want %q", got, "find-window")
	}
}

func TestCommandMode_TabCompletion_NoMatch(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{
		Complete: func(_ string) []string { return nil },
	})
	m.Key(pressRune('z'))
	m.Key(press(keys.CodeTab)) // no completions; buffer unchanged
	if got := m.Input(); got != "z" {
		t.Errorf("Input() = %q, want %q", got, "z")
	}
}

func TestCommandMode_TabCompletion_NilComplete(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	m.Key(pressRune('a'))
	m.Key(press(keys.CodeTab)) // no-op when Complete is nil
	if got := m.Input(); got != "a" {
		t.Errorf("Input() = %q, want %q", got, "a")
	}
}

func TestCommandMode_TabCompletion_ResetOnOtherKey(t *testing.T) {
	calls := 0
	m := prompt.NewCommand(defaultRect(), prompt.Config{
		Complete: func(_ string) []string {
			calls++
			return []string{"alpha", "beta"}
		},
	})
	m.Key(pressRune('a'))
	m.Key(press(keys.CodeTab)) // first cycle; calls Complete once
	m.Key(pressRune('x'))      // resets completion
	m.Key(press(keys.CodeTab)) // starts a new cycle; calls Complete again
	if calls != 2 {
		t.Errorf("Complete called %d times, want 2", calls)
	}
}

func TestCommandMode_TabCompletion_PreservesSuffix(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{
		Complete: func(partial string) []string {
			if partial == "fi" {
				return []string{"find"}
			}
			return nil
		},
	})
	// type "fi nish" then move cursor back to position 2
	for _, r := range "finish" {
		m.Key(pressRune(r))
	}
	// move cursor left 4 positions so cursor is at 2 ("fi|nish")
	for i := 0; i < 4; i++ {
		m.Key(press(keys.CodeLeft))
	}
	m.Key(press(keys.CodeTab))
	// "fi" → "find"; suffix "nish" is preserved → "findnish"
	if got := m.Input(); got != "findnish" {
		t.Errorf("Input() = %q, want %q", got, "findnish")
	}
}

// ============================================================================
// CommandMode — confirm and cancel
// ============================================================================

func TestCommandMode_Confirm(t *testing.T) {
	var confirmed string
	m := prompt.NewCommand(defaultRect(), prompt.Config{
		OnConfirm: func(input string) { confirmed = input },
	})
	m.Key(pressRune('h'))
	m.Key(pressRune('i'))
	outcome := m.Key(press(keys.CodeEnter))
	if confirmed != "hi" {
		t.Errorf("OnConfirm got %q, want %q", confirmed, "hi")
	}
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("outcome.Kind = %v, want KindCloseMode", outcome.Kind)
	}
}

func TestCommandMode_ConfirmTemplate(t *testing.T) {
	var confirmed string
	m := prompt.NewCommand(defaultRect(), prompt.Config{
		Template:  "find-window '%%'",
		OnConfirm: func(input string) { confirmed = input },
	})
	for _, r := range "main" {
		m.Key(pressRune(r))
	}
	m.Key(press(keys.CodeEnter))
	if confirmed != "find-window 'main'" {
		t.Errorf("OnConfirm got %q, want %q", confirmed, "find-window 'main'")
	}
}

func TestCommandMode_ConfirmNilCallback(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	// must not panic when OnConfirm is nil
	outcome := m.Key(press(keys.CodeEnter))
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("outcome.Kind = %v, want KindCloseMode", outcome.Kind)
	}
}

func TestCommandMode_Cancel(t *testing.T) {
	cancelled := false
	m := prompt.NewCommand(defaultRect(), prompt.Config{
		OnCancel: func() { cancelled = true },
	})
	outcome := m.Key(press(keys.CodeEscape))
	if !cancelled {
		t.Error("OnCancel not called")
	}
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("outcome.Kind = %v, want KindCloseMode", outcome.Kind)
	}
}

func TestCommandMode_CancelNilCallback(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	// must not panic when OnCancel is nil
	outcome := m.Key(press(keys.CodeEscape))
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("outcome.Kind = %v, want KindCloseMode", outcome.Kind)
	}
}

// ============================================================================
// CommandMode — interface methods
// ============================================================================

func TestCommandMode_CaptureFocus(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	if !m.CaptureFocus() {
		t.Error("CaptureFocus() = false, want true")
	}
}

func TestCommandMode_MousePassthrough(t *testing.T) {
	m := prompt.NewCommand(defaultRect(), prompt.Config{})
	outcome := m.Mouse(keys.MouseEvent{})
	if outcome.Kind != modes.KindPassthrough {
		t.Errorf("Mouse().Kind = %v, want KindPassthrough", outcome.Kind)
	}
}

func TestCommandMode_Render(t *testing.T) {
	rect := modes.Rect{X: 0, Y: 0, Width: 20, Height: 1}
	m := prompt.NewCommand(rect, prompt.Config{Prompt: "> "})
	m.Key(pressRune('h'))
	m.Key(pressRune('i'))
	dst := make([]modes.Cell, 20)
	m.Render(dst)
	if dst[0].Char != '>' || dst[1].Char != ' ' {
		t.Errorf("prompt prefix: got %c%c, want '> '", dst[0].Char, dst[1].Char)
	}
	if dst[2].Char != 'h' || dst[3].Char != 'i' {
		t.Errorf("input chars: got %c%c, want 'hi'", dst[2].Char, dst[3].Char)
	}
	// cursor indicator appears after input
	if dst[4].Char != '_' {
		t.Errorf("cursor indicator: dst[4].Char = %c, want '_'", dst[4].Char)
	}
}

func TestCommandMode_Rect(t *testing.T) {
	rect := modes.Rect{X: 5, Y: 10, Width: 60, Height: 1}
	m := prompt.NewCommand(rect, prompt.Config{})
	if got := m.Rect(); got != rect {
		t.Errorf("Rect() = %+v, want %+v", got, rect)
	}
}

// ============================================================================
// ConfirmMode tests
// ============================================================================

func TestConfirmMode_Yes_y(t *testing.T) {
	confirmed := false
	m := prompt.NewConfirm(defaultRect(), "Are you sure? ", func() { confirmed = true }, nil)
	outcome := m.Key(pressRune('y'))
	if !confirmed {
		t.Error("onYes not called for 'y'")
	}
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("outcome.Kind = %v, want KindCloseMode", outcome.Kind)
	}
}

func TestConfirmMode_Yes_Y(t *testing.T) {
	confirmed := false
	m := prompt.NewConfirm(defaultRect(), "Are you sure? ", func() { confirmed = true }, nil)
	m.Key(pressRune('Y'))
	if !confirmed {
		t.Error("onYes not called for 'Y'")
	}
}

func TestConfirmMode_Yes_Enter(t *testing.T) {
	confirmed := false
	m := prompt.NewConfirm(defaultRect(), "Are you sure? ", func() { confirmed = true }, nil)
	m.Key(press(keys.CodeEnter))
	if !confirmed {
		t.Error("onYes not called for Enter")
	}
}

func TestConfirmMode_No_n(t *testing.T) {
	cancelled := false
	m := prompt.NewConfirm(defaultRect(), "Are you sure? ", nil, func() { cancelled = true })
	outcome := m.Key(pressRune('n'))
	if !cancelled {
		t.Error("onNo not called for 'n'")
	}
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("outcome.Kind = %v, want KindCloseMode", outcome.Kind)
	}
}

func TestConfirmMode_No_Escape(t *testing.T) {
	cancelled := false
	m := prompt.NewConfirm(defaultRect(), "Are you sure? ", nil, func() { cancelled = true })
	m.Key(press(keys.CodeEscape))
	if !cancelled {
		t.Error("onNo not called for Escape")
	}
}

func TestConfirmMode_NilCallbacks(t *testing.T) {
	m := prompt.NewConfirm(defaultRect(), "prompt", nil, nil)
	// must not panic
	m.Key(pressRune('y'))
	m.Key(pressRune('n'))
}

func TestConfirmMode_CaptureFocus(t *testing.T) {
	m := prompt.NewConfirm(defaultRect(), "prompt", nil, nil)
	if !m.CaptureFocus() {
		t.Error("CaptureFocus() = false, want true")
	}
}

func TestConfirmMode_MousePassthrough(t *testing.T) {
	m := prompt.NewConfirm(defaultRect(), "prompt", nil, nil)
	outcome := m.Mouse(keys.MouseEvent{})
	if outcome.Kind != modes.KindPassthrough {
		t.Errorf("Mouse().Kind = %v, want KindPassthrough", outcome.Kind)
	}
}

func TestConfirmMode_Render(t *testing.T) {
	rect := modes.Rect{X: 0, Y: 0, Width: 20, Height: 1}
	m := prompt.NewConfirm(rect, "Sure? ", nil, nil)
	dst := make([]modes.Cell, 20)
	m.Render(dst)
	expected := []rune("Sure? ")
	for i, r := range expected {
		if dst[i].Char != r {
			t.Errorf("dst[%d].Char = %c, want %c", i, dst[i].Char, r)
		}
	}
	// remainder should be spaces
	for i := len(expected); i < len(dst); i++ {
		if dst[i].Char != ' ' {
			t.Errorf("dst[%d].Char = %c, want ' '", i, dst[i].Char)
		}
	}
}

func TestConfirmMode_Rect(t *testing.T) {
	rect := modes.Rect{X: 0, Y: 24, Width: 80, Height: 1}
	m := prompt.NewConfirm(rect, "prompt", nil, nil)
	if got := m.Rect(); got != rect {
		t.Errorf("Rect() = %+v, want %+v", got, rect)
	}
}
