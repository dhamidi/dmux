package modes_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func sampleOptions() []modes.CustomizeOptionEntry {
	return []modes.CustomizeOptionEntry{
		{Scope: "server", Name: "escape-time", Value: "500"},
		{Scope: "session", Name: "status", Value: "on"},
		{Scope: "window", Name: "mode-keys", Value: "vi"},
	}
}

func sampleBindings() []modes.CustomizeBindingEntry {
	return []modes.CustomizeBindingEntry{
		{Table: "root", Key: "C-b", Command: "send-prefix"},
		{Table: "prefix", Key: "c", Command: "new-window"},
	}
}

func newOverlay() *modes.CustomizeOverlay {
	return modes.NewCustomizeOverlay(
		modes.Rect{X: 0, Y: 0, Width: 80, Height: 24},
		sampleOptions(),
		sampleBindings(),
		nil,
		nil,
	)
}

func renderOverlay(m *modes.CustomizeOverlay) []modes.Cell {
	r := m.Rect()
	dst := make([]modes.Cell, r.Width*r.Height)
	m.Render(dst)
	return dst
}

// ── interface compliance ─────────────────────────────────────────────────────

func TestCustomizeOverlay_ImplementsClientOverlay(t *testing.T) {
	var _ modes.ClientOverlay = newOverlay()
}

// ── construction ─────────────────────────────────────────────────────────────

func TestCustomizeOverlay_RendersWithoutError(t *testing.T) {
	m := newOverlay()
	dst := renderOverlay(m)
	if len(dst) != 80*24 {
		t.Errorf("Render dst length = %d, want %d", len(dst), 80*24)
	}
}

func TestCustomizeOverlay_EmptyOptionsAndBindings(t *testing.T) {
	m := modes.NewCustomizeOverlay(
		modes.Rect{X: 0, Y: 0, Width: 40, Height: 10},
		nil,
		nil,
		nil,
		nil,
	)
	dst := make([]modes.Cell, 40*10)
	m.Render(dst) // must not panic
}

func TestCustomizeOverlay_HasVisibleRows(t *testing.T) {
	m := newOverlay()
	if m.FlatLen() == 0 {
		t.Error("expected at least one visible row, got 0")
	}
}

// ── navigation ───────────────────────────────────────────────────────────────

func TestCustomizeOverlay_InitialCursorIsZero(t *testing.T) {
	m := newOverlay()
	if got := m.Cursor(); got != 0 {
		t.Errorf("initial Cursor() = %d, want 0", got)
	}
}

func TestCustomizeOverlay_DownMovesSelection(t *testing.T) {
	m := newOverlay()
	out := m.Key(keys.Key{Code: keys.CodeDown})
	if out.Kind != modes.KindConsumed {
		t.Errorf("Down: want KindConsumed, got %v", out.Kind)
	}
	if got := m.Cursor(); got != 1 {
		t.Errorf("after Down: Cursor() = %d, want 1", got)
	}
}

func TestCustomizeOverlay_UpMovesSelection(t *testing.T) {
	m := newOverlay()
	m.Key(keys.Key{Code: keys.CodeDown}) // move to 1
	out := m.Key(keys.Key{Code: keys.CodeUp})
	if out.Kind != modes.KindConsumed {
		t.Errorf("Up: want KindConsumed, got %v", out.Kind)
	}
	if got := m.Cursor(); got != 0 {
		t.Errorf("after Down+Up: Cursor() = %d, want 0", got)
	}
}

func TestCustomizeOverlay_DownDoesNotGoNegative(t *testing.T) {
	m := newOverlay()
	// Press Up from position 0 — cursor should stay at 0.
	m.Key(keys.Key{Code: keys.CodeUp})
	if got := m.Cursor(); got != 0 {
		t.Errorf("Up at top: Cursor() = %d, want 0 (no wrap)", got)
	}
}

func TestCustomizeOverlay_DownClampsAtEnd(t *testing.T) {
	m := newOverlay()
	n := m.FlatLen()
	// Press Down more times than there are rows.
	for i := 0; i < n+5; i++ {
		m.Key(keys.Key{Code: keys.CodeDown})
	}
	if got := m.Cursor(); got != n-1 {
		t.Errorf("after many Downs: Cursor() = %d, want %d (last row)", got, n-1)
	}
}

func TestCustomizeOverlay_VimKeysJK(t *testing.T) {
	m := newOverlay()
	m.Key(keys.Key{Code: keys.KeyCode('j')})
	if got := m.Cursor(); got != 1 {
		t.Errorf("after j: Cursor() = %d, want 1", got)
	}
	m.Key(keys.Key{Code: keys.KeyCode('k')})
	if got := m.Cursor(); got != 0 {
		t.Errorf("after j+k: Cursor() = %d, want 0", got)
	}
}

// ── collapse / expand ────────────────────────────────────────────────────────

func TestCustomizeOverlay_RightExpandsGroup(t *testing.T) {
	m := modes.NewCustomizeOverlay(
		modes.Rect{X: 0, Y: 0, Width: 40, Height: 20},
		sampleOptions(),
		nil,
		nil,
		nil,
	)
	// Collapse the top-level Options group first.
	m.Key(keys.Key{Code: keys.CodeLeft})
	collapsedLen := m.FlatLen()
	// Expand it back.
	m.Key(keys.Key{Code: keys.CodeRight})
	expandedLen := m.FlatLen()
	if expandedLen <= collapsedLen {
		t.Errorf("after Right expand: FlatLen %d <= collapsed %d, expected more rows", expandedLen, collapsedLen)
	}
}

func TestCustomizeOverlay_LeftCollapsesGroup(t *testing.T) {
	m := newOverlay()
	before := m.FlatLen()
	// Cursor is at top-level "Options" group; collapse it.
	m.Key(keys.Key{Code: keys.CodeLeft})
	after := m.FlatLen()
	if after >= before {
		t.Errorf("after Left collapse: FlatLen %d >= before %d, expected fewer rows", after, before)
	}
}

func TestCustomizeOverlay_EnterTogglesGroup(t *testing.T) {
	m := newOverlay()
	before := m.FlatLen()
	// Enter on the "Options" top-level group should collapse it.
	m.Key(keys.Key{Code: keys.CodeEnter})
	after := m.FlatLen()
	if after >= before {
		t.Errorf("Enter on group: FlatLen %d >= before %d, expected fewer rows", after, before)
	}
}

// ── edit mode ────────────────────────────────────────────────────────────────

func TestCustomizeOverlay_EnterOnOptionEntersEditMode(t *testing.T) {
	m := newOverlay()
	// Navigate to the first option leaf.
	// Tree: Options(0) > server(1) > escape-time(2) > ...
	m.Key(keys.Key{Code: keys.CodeDown}) // cursor=1 (server group)
	m.Key(keys.Key{Code: keys.CodeDown}) // cursor=2 (escape-time leaf)
	out := m.Key(keys.Key{Code: keys.CodeEnter})
	if out.Kind != modes.KindConsumed {
		t.Errorf("Enter on option: want KindConsumed, got %v", out.Kind)
	}
	if !m.Editing() {
		t.Error("Enter on option: expected Editing() == true")
	}
}

func TestCustomizeOverlay_EscapeInEditModeCancels(t *testing.T) {
	m := newOverlay()
	// Navigate to an option leaf.
	m.Key(keys.Key{Code: keys.CodeDown})
	m.Key(keys.Key{Code: keys.CodeDown})
	m.Key(keys.Key{Code: keys.CodeEnter}) // enter edit mode
	if !m.Editing() {
		t.Skip("not in edit mode, skipping")
	}
	out := m.Key(keys.Key{Code: keys.CodeEscape})
	if out.Kind != modes.KindConsumed {
		t.Errorf("Escape in edit: want KindConsumed, got %v", out.Kind)
	}
	if m.Editing() {
		t.Error("Escape in edit: expected Editing() == false after Escape")
	}
}

func TestCustomizeOverlay_EnterCommitsEdit(t *testing.T) {
	var setOptionCalled bool
	var setOptionValue string
	m := modes.NewCustomizeOverlay(
		modes.Rect{X: 0, Y: 0, Width: 80, Height: 24},
		[]modes.CustomizeOptionEntry{
			{Scope: "server", Name: "escape-time", Value: "500"},
		},
		nil,
		func(scope, name, value string) error {
			setOptionCalled = true
			setOptionValue = value
			return nil
		},
		nil,
	)
	// Navigate to the option leaf: Options(0) > server(1) > escape-time(2).
	m.Key(keys.Key{Code: keys.CodeDown})
	m.Key(keys.Key{Code: keys.CodeDown})
	m.Key(keys.Key{Code: keys.CodeEnter}) // enter edit mode
	if !m.Editing() {
		t.Skip("not in edit mode")
	}
	// Clear the pre-populated value ("500" = 3 chars) then type "100".
	m.Key(keys.Key{Code: keys.CodeBackspace})
	m.Key(keys.Key{Code: keys.CodeBackspace})
	m.Key(keys.Key{Code: keys.CodeBackspace})
	m.Key(keys.Key{Code: keys.KeyCode('1')})
	m.Key(keys.Key{Code: keys.KeyCode('0')})
	m.Key(keys.Key{Code: keys.KeyCode('0')})
	// Commit.
	m.Key(keys.Key{Code: keys.CodeEnter})
	if m.Editing() {
		t.Error("after committing edit: Editing() should be false")
	}
	if !setOptionCalled {
		t.Error("setOption callback was not called")
	}
	if setOptionValue != "100" {
		t.Errorf("setOption called with value %q, want %q", setOptionValue, "100")
	}
}

// ── filter mode ──────────────────────────────────────────────────────────────

func TestCustomizeOverlay_SlashEntersFilterMode(t *testing.T) {
	m := newOverlay()
	m.Key(keys.Key{Code: keys.KeyCode('/')})
	if !m.Filtering() {
		t.Error("/ key: expected Filtering() == true")
	}
}

func TestCustomizeOverlay_EscapeExitsFilterMode(t *testing.T) {
	m := newOverlay()
	m.Key(keys.Key{Code: keys.KeyCode('/')})
	m.Key(keys.Key{Code: keys.CodeEscape})
	if m.Filtering() {
		t.Error("Escape in filter mode: expected Filtering() == false")
	}
}

// ── close ────────────────────────────────────────────────────────────────────

func TestCustomizeOverlay_EscapeInNavigationClosesOverlay(t *testing.T) {
	m := newOverlay()
	out := m.Key(keys.Key{Code: keys.CodeEscape})
	if out.Kind != modes.KindCloseMode {
		t.Errorf("Escape in nav: want KindCloseMode, got %v", out.Kind)
	}
}

func TestCustomizeOverlay_QClosesOverlay(t *testing.T) {
	m := newOverlay()
	out := m.Key(keys.Key{Code: keys.KeyCode('q')})
	if out.Kind != modes.KindCloseMode {
		t.Errorf("q: want KindCloseMode, got %v", out.Kind)
	}
}

func TestCustomizeOverlay_CaptureFocusReturnsTrue(t *testing.T) {
	m := newOverlay()
	if !m.CaptureFocus() {
		t.Error("CaptureFocus() should return true")
	}
}

func TestCustomizeOverlay_CloseDoesNotPanic(t *testing.T) {
	m := newOverlay()
	m.Close() // must not panic
}

// ── mouse ─────────────────────────────────────────────────────────────────────

func TestCustomizeOverlay_MouseClickOutsidePassesThrough(t *testing.T) {
	m := modes.NewCustomizeOverlay(
		modes.Rect{X: 10, Y: 5, Width: 40, Height: 20},
		sampleOptions(),
		nil,
		nil,
		nil,
	)
	ev := keys.MouseEvent{Action: keys.MousePress, Button: keys.MouseLeft, Col: 0, Row: 0}
	out := m.Mouse(ev)
	if out.Kind != modes.KindPassthrough {
		t.Errorf("click outside: want KindPassthrough, got %v", out.Kind)
	}
}

func TestCustomizeOverlay_MouseClickInsideUpdatesCursor(t *testing.T) {
	m := modes.NewCustomizeOverlay(
		modes.Rect{X: 0, Y: 0, Width: 40, Height: 20},
		sampleOptions(),
		nil,
		nil,
		nil,
	)
	// Click on row 2 (0-based).
	ev := keys.MouseEvent{Action: keys.MouseMotion, Col: 5, Row: 2}
	out := m.Mouse(ev)
	if out.Kind != modes.KindConsumed {
		t.Errorf("motion inside: want KindConsumed, got %v", out.Kind)
	}
	if m.Cursor() != 2 {
		t.Errorf("after motion row 2: Cursor() = %d, want 2", m.Cursor())
	}
}

// ── render content ────────────────────────────────────────────────────────────

func TestCustomizeOverlay_RenderShowsOptionsHeader(t *testing.T) {
	m := newOverlay()
	dst := renderOverlay(m)
	w := m.Rect().Width
	// Row 0 should contain "Options".
	row0 := make([]rune, w)
	for col := 0; col < w; col++ {
		ch := dst[col].Char
		if ch == 0 {
			ch = ' '
		}
		row0[col] = ch
	}
	if !containsRune(string(row0), "Options") {
		t.Errorf("row 0 = %q, expected to contain 'Options'", string(row0))
	}
}

func containsRune(s, substr string) bool {
	return len(s) >= len(substr) && func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}()
}
