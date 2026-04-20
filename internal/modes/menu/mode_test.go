package menu_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	"github.com/dhamidi/dmux/internal/modes/menu"
)

// ---- interface compliance --------------------------------------------------

func TestImplementsClientOverlay(t *testing.T) {
	m := menu.New(modes.Rect{}, nil)
	var _ modes.ClientOverlay = m
}

// ---- helpers ---------------------------------------------------------------

func makeMenu(items []menu.MenuItem) *menu.Mode {
	return menu.New(modes.Rect{X: 0, Y: 0, Width: 1, Height: 1}, items)
}

func threeItems() []menu.MenuItem {
	return []menu.MenuItem{
		{Label: "Alpha", Mnemonic: 'a', Enabled: true},
		{Label: "Beta", Mnemonic: 'b', Enabled: true},
		{Label: "Gamma", Mnemonic: 'g', Enabled: true},
	}
}

func renderMenu(m *menu.Mode) []modes.Cell {
	r := m.Rect()
	dst := make([]modes.Cell, r.Width*r.Height)
	m.Render(dst)
	return dst
}

func cellString(dst []modes.Cell, row, width int) string {
	runes := make([]rune, width)
	for col := 0; col < width; col++ {
		ch := dst[row*width+col].Char
		if ch == 0 {
			ch = ' '
		}
		runes[col] = ch
	}
	return string(runes)
}

// ---- construction ----------------------------------------------------------

func TestNew_SelectsFirstEnabledItem(t *testing.T) {
	m := makeMenu(threeItems())
	if got := m.Selected(); got != 0 {
		t.Errorf("Selected() = %d, want 0", got)
	}
}

func TestNew_SkipsSeparatorForInitialSelection(t *testing.T) {
	items := []menu.MenuItem{
		{Separator: true},
		{Label: "One", Enabled: true},
	}
	m := makeMenu(items)
	if got := m.Selected(); got != 1 {
		t.Errorf("Selected() = %d, want 1 (skipping separator)", got)
	}
}

func TestNew_SkipsDisabledForInitialSelection(t *testing.T) {
	items := []menu.MenuItem{
		{Label: "Disabled", Enabled: false},
		{Label: "Enabled", Enabled: true},
	}
	m := makeMenu(items)
	if got := m.Selected(); got != 1 {
		t.Errorf("Selected() = %d, want 1 (skipping disabled)", got)
	}
}

func TestNew_NoSelectableItems(t *testing.T) {
	items := []menu.MenuItem{
		{Separator: true},
		{Label: "Disabled", Enabled: false},
	}
	m := makeMenu(items)
	if got := m.Selected(); got != -1 {
		t.Errorf("Selected() = %d, want -1 (no selectable items)", got)
	}
}

func TestNew_EmptyItemList(t *testing.T) {
	m := makeMenu(nil)
	if got := m.Selected(); got != -1 {
		t.Errorf("Selected() = %d, want -1", got)
	}
}

// ---- Rect ------------------------------------------------------------------

func TestRect_WidthAccommodatesLongestLabel(t *testing.T) {
	items := []menu.MenuItem{
		{Label: "Short", Enabled: true},
		{Label: "Much longer label", Enabled: true},
	}
	m := makeMenu(items)
	r := m.Rect()
	// Width = len("Much longer label") + 2 (prefix) = 19
	want := len([]rune("Much longer label")) + 2
	if r.Width != want {
		t.Errorf("Rect().Width = %d, want %d", r.Width, want)
	}
}

func TestRect_HeightEqualsItemCount(t *testing.T) {
	items := threeItems()
	m := makeMenu(items)
	r := m.Rect()
	if r.Height != len(items) {
		t.Errorf("Rect().Height = %d, want %d", r.Height, len(items))
	}
}

func TestRect_AnchorPositionPreserved(t *testing.T) {
	anchor := modes.Rect{X: 10, Y: 5}
	m := menu.New(anchor, threeItems())
	r := m.Rect()
	if r.X != anchor.X || r.Y != anchor.Y {
		t.Errorf("Rect() top-left = (%d,%d), want (%d,%d)", r.X, r.Y, anchor.X, anchor.Y)
	}
}

// ---- rendering -------------------------------------------------------------

func TestRender_SelectedItemHasMarker(t *testing.T) {
	m := makeMenu(threeItems())
	dst := renderMenu(m)
	w := m.Rect().Width
	// Row 0 should have '>' as first char.
	if got := dst[0].Char; got != '>' {
		t.Errorf("Render row 0 col 0 = %q, want '>'", got)
	}
	// Row 1 should have ' ' as first char.
	if got := dst[1*w].Char; got != ' ' && got != 0 {
		t.Errorf("Render row 1 col 0 = %q, want ' '", got)
	}
}

func TestRender_LabelAppearsAfterPrefix(t *testing.T) {
	items := []menu.MenuItem{{Label: "Hello", Enabled: true}}
	m := makeMenu(items)
	dst := renderMenu(m)
	w := m.Rect().Width
	s := cellString(dst, 0, w)
	// Should start with "> Hello"
	if len(s) < 7 || s[:7] != "> Hello" {
		t.Errorf("Render row 0 = %q, want prefix \"> Hello\"", s)
	}
}

func TestRender_SeparatorFillsRowWithHorizontalRule(t *testing.T) {
	items := []menu.MenuItem{
		{Label: "Above", Enabled: true},
		{Separator: true},
		{Label: "Below", Enabled: true},
	}
	m := makeMenu(items)
	dst := renderMenu(m)
	w := m.Rect().Width
	// Row 1 should be all '─'.
	for col := 0; col < w; col++ {
		if got := dst[1*w+col].Char; got != '─' {
			t.Errorf("Render separator row 1 col %d = %q, want '─'", col, got)
		}
	}
}

func TestRender_DisabledItemRenderedWithoutMarker(t *testing.T) {
	items := []menu.MenuItem{
		{Label: "Disabled", Enabled: false},
		{Label: "Enabled", Enabled: true},
	}
	m := makeMenu(items)
	dst := renderMenu(m)
	w := m.Rect().Width
	// Row 0 (disabled, not selected) should have ' ' as marker.
	if got := dst[0].Char; got != ' ' && got != 0 {
		t.Errorf("Render disabled row col 0 = %q, want ' '", got)
	}
	// Row 1 (selected) should have '>'.
	if got := dst[1*w].Char; got != '>' {
		t.Errorf("Render selected row col 0 = %q, want '>'", got)
	}
}

// ---- navigation: arrow keys ------------------------------------------------

func TestKey_DownMovesToNextItem(t *testing.T) {
	m := makeMenu(threeItems())
	// Initial selection is 0.
	out := m.Key(keys.Key{Code: keys.CodeDown})
	if out.Kind != modes.KindConsumed {
		t.Errorf("Down: want KindConsumed, got %v", out.Kind)
	}
	if got := m.Selected(); got != 1 {
		t.Errorf("After Down: Selected() = %d, want 1", got)
	}
}

func TestKey_UpMovesToPrevItem(t *testing.T) {
	m := makeMenu(threeItems())
	// Move to item 1 first.
	m.Key(keys.Key{Code: keys.CodeDown})
	out := m.Key(keys.Key{Code: keys.CodeUp})
	if out.Kind != modes.KindConsumed {
		t.Errorf("Up: want KindConsumed, got %v", out.Kind)
	}
	if got := m.Selected(); got != 0 {
		t.Errorf("After Down+Up: Selected() = %d, want 0", got)
	}
}

func TestKey_DownWrapsAround(t *testing.T) {
	items := threeItems()
	m := makeMenu(items)
	// Move to last item.
	m.Key(keys.Key{Code: keys.CodeDown})
	m.Key(keys.Key{Code: keys.CodeDown})
	// Now wrap around.
	m.Key(keys.Key{Code: keys.CodeDown})
	if got := m.Selected(); got != 0 {
		t.Errorf("After wrapping Down: Selected() = %d, want 0", got)
	}
}

func TestKey_UpWrapsAround(t *testing.T) {
	m := makeMenu(threeItems())
	// At item 0, press Up — should wrap to last item (2).
	m.Key(keys.Key{Code: keys.CodeUp})
	if got := m.Selected(); got != 2 {
		t.Errorf("After wrapping Up: Selected() = %d, want 2", got)
	}
}

func TestKey_NavigationSkipsSeparators(t *testing.T) {
	items := []menu.MenuItem{
		{Label: "First", Enabled: true},
		{Separator: true},
		{Label: "Third", Enabled: true},
	}
	m := makeMenu(items)
	// Down from 0 should jump over separator to 2.
	m.Key(keys.Key{Code: keys.CodeDown})
	if got := m.Selected(); got != 2 {
		t.Errorf("After Down (skipping separator): Selected() = %d, want 2", got)
	}
}

func TestKey_NavigationSkipsDisabledItems(t *testing.T) {
	items := []menu.MenuItem{
		{Label: "Enabled", Enabled: true},
		{Label: "Disabled", Enabled: false},
		{Label: "Enabled2", Enabled: true},
	}
	m := makeMenu(items)
	m.Key(keys.Key{Code: keys.CodeDown})
	if got := m.Selected(); got != 2 {
		t.Errorf("After Down (skipping disabled): Selected() = %d, want 2", got)
	}
}

// ---- selection: Enter ------------------------------------------------------

func TestKey_EnterCallsOnSelectAndCloses(t *testing.T) {
	called := false
	items := []menu.MenuItem{
		{Label: "Item", Enabled: true, OnSelect: func() { called = true }},
	}
	m := makeMenu(items)
	out := m.Key(keys.Key{Code: keys.CodeEnter})
	if out.Kind != modes.KindCloseMode {
		t.Errorf("Enter: want KindCloseMode, got %v", out.Kind)
	}
	if !called {
		t.Error("Enter: OnSelect was not called")
	}
}

func TestKey_EnterWithNilOnSelectDoesNotPanic(t *testing.T) {
	items := []menu.MenuItem{
		{Label: "Item", Enabled: true, OnSelect: nil},
	}
	m := makeMenu(items)
	out := m.Key(keys.Key{Code: keys.CodeEnter})
	if out.Kind != modes.KindCloseMode {
		t.Errorf("Enter (nil OnSelect): want KindCloseMode, got %v", out.Kind)
	}
}

func TestKey_EnterWhenNoSelectionCloses(t *testing.T) {
	// All items non-selectable → selected == -1.
	items := []menu.MenuItem{{Separator: true}}
	m := makeMenu(items)
	out := m.Key(keys.Key{Code: keys.CodeEnter})
	if out.Kind != modes.KindCloseMode {
		t.Errorf("Enter (no selection): want KindCloseMode, got %v", out.Kind)
	}
}

// ---- selection: Escape -----------------------------------------------------

func TestKey_EscapeClosesMenu(t *testing.T) {
	m := makeMenu(threeItems())
	out := m.Key(keys.Key{Code: keys.CodeEscape})
	if out.Kind != modes.KindCloseMode {
		t.Errorf("Escape: want KindCloseMode, got %v", out.Kind)
	}
}

func TestKey_EscapeDoesNotCallOnSelect(t *testing.T) {
	called := false
	items := []menu.MenuItem{
		{Label: "Item", Enabled: true, OnSelect: func() { called = true }},
	}
	m := makeMenu(items)
	m.Key(keys.Key{Code: keys.CodeEscape})
	if called {
		t.Error("Escape: OnSelect must not be called")
	}
}

// ---- selection: mnemonics --------------------------------------------------

func TestKey_MnemonicActivatesMatchingItem(t *testing.T) {
	called := false
	items := []menu.MenuItem{
		{Label: "Alpha", Mnemonic: 'a', Enabled: true},
		{Label: "Beta", Mnemonic: 'b', Enabled: true, OnSelect: func() { called = true }},
	}
	m := makeMenu(items)
	out := m.Key(keys.Key{Code: keys.KeyCode('b')})
	if out.Kind != modes.KindCloseMode {
		t.Errorf("Mnemonic 'b': want KindCloseMode, got %v", out.Kind)
	}
	if !called {
		t.Error("Mnemonic 'b': OnSelect not called")
	}
}

func TestKey_MnemonicIgnoresDisabledItem(t *testing.T) {
	called := false
	items := []menu.MenuItem{
		{Label: "Disabled", Mnemonic: 'd', Enabled: false, OnSelect: func() { called = true }},
		{Label: "Other", Mnemonic: 'o', Enabled: true},
	}
	m := makeMenu(items)
	out := m.Key(keys.Key{Code: keys.KeyCode('d')})
	// Should not activate — key consumed with no close.
	if out.Kind == modes.KindCloseMode {
		t.Errorf("Mnemonic 'd' (disabled): want !KindCloseMode, got KindCloseMode")
	}
	if called {
		t.Error("Mnemonic 'd' (disabled): OnSelect must not be called")
	}
}

func TestKey_UnknownKeyConsumed(t *testing.T) {
	m := makeMenu(threeItems())
	out := m.Key(keys.Key{Code: keys.KeyCode('z')})
	if out.Kind != modes.KindConsumed {
		t.Errorf("Unknown key 'z': want KindConsumed, got %v", out.Kind)
	}
}

// ---- mouse -----------------------------------------------------------------

func TestMouse_MotionInsideMenuUpdatesSelection(t *testing.T) {
	anchor := modes.Rect{X: 5, Y: 10}
	m := menu.New(anchor, threeItems())
	// Hover over row 1 (screen row 11).
	ev := keys.MouseEvent{
		Action: keys.MouseMotion,
		Col:    5,
		Row:    11,
	}
	out := m.Mouse(ev)
	if out.Kind != modes.KindConsumed {
		t.Errorf("Mouse motion inside: want KindConsumed, got %v", out.Kind)
	}
	if got := m.Selected(); got != 1 {
		t.Errorf("After mouse hover row 1: Selected() = %d, want 1", got)
	}
}

func TestMouse_ClickInsideMenuActivatesItem(t *testing.T) {
	called := false
	items := []menu.MenuItem{
		{Label: "One", Enabled: true},
		{Label: "Two", Enabled: true, OnSelect: func() { called = true }},
	}
	anchor := modes.Rect{X: 0, Y: 0}
	m := menu.New(anchor, items)
	ev := keys.MouseEvent{
		Action: keys.MousePress,
		Button: keys.MouseLeft,
		Col:    0,
		Row:    1,
	}
	out := m.Mouse(ev)
	if out.Kind != modes.KindCloseMode {
		t.Errorf("Mouse click row 1: want KindCloseMode, got %v", out.Kind)
	}
	if !called {
		t.Error("Mouse click row 1: OnSelect not called")
	}
}

func TestMouse_ClickOutsideMenuPassesThrough(t *testing.T) {
	anchor := modes.Rect{X: 10, Y: 10}
	m := menu.New(anchor, threeItems())
	ev := keys.MouseEvent{
		Action: keys.MousePress,
		Button: keys.MouseLeft,
		Col:    0,
		Row:    0,
	}
	out := m.Mouse(ev)
	if out.Kind != modes.KindPassthrough {
		t.Errorf("Mouse click outside: want KindPassthrough, got %v", out.Kind)
	}
}

func TestMouse_ClickOnSeparatorConsumed(t *testing.T) {
	items := []menu.MenuItem{
		{Label: "Above", Enabled: true},
		{Separator: true},
		{Label: "Below", Enabled: true},
	}
	m := menu.New(modes.Rect{}, items)
	ev := keys.MouseEvent{
		Action: keys.MousePress,
		Button: keys.MouseLeft,
		Col:    0,
		Row:    1, // separator row
	}
	out := m.Mouse(ev)
	if out.Kind != modes.KindConsumed {
		t.Errorf("Click on separator: want KindConsumed, got %v", out.Kind)
	}
}

func TestMouse_ClickOnDisabledItemConsumed(t *testing.T) {
	called := false
	items := []menu.MenuItem{
		{Label: "Disabled", Enabled: false, OnSelect: func() { called = true }},
	}
	m := menu.New(modes.Rect{}, items)
	ev := keys.MouseEvent{
		Action: keys.MousePress,
		Button: keys.MouseLeft,
		Col:    0,
		Row:    0,
	}
	out := m.Mouse(ev)
	if out.Kind == modes.KindCloseMode {
		t.Errorf("Click on disabled: want !KindCloseMode, got KindCloseMode")
	}
	if called {
		t.Error("Click on disabled: OnSelect must not be called")
	}
}

// ---- CaptureFocus ----------------------------------------------------------

func TestCaptureFocus_ReturnsTrue(t *testing.T) {
	m := makeMenu(threeItems())
	if !m.CaptureFocus() {
		t.Error("CaptureFocus() should return true")
	}
}

// ---- Close -----------------------------------------------------------------

func TestClose_DoesNotPanic(t *testing.T) {
	m := makeMenu(threeItems())
	m.Close() // must not panic
}
