package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/command"
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	menumode "github.com/dhamidi/dmux/internal/modes/menu"
	"github.com/dhamidi/dmux/internal/session"
)

// newTestMutatorWithDisplayMenu creates a minimal serverMutator suitable for
// display-menu tests. It registers client "c1" with a 24×80 terminal and
// wires overlay push/pop callbacks for inspection.
func newTestMutatorWithDisplayMenu() (
	m *serverMutator,
	pushed func() modes.ClientOverlay,
) {
	state := session.NewServer()

	c := session.NewClient(session.ClientID("c1"))
	c.Size = session.Size{Rows: 24, Cols: 80}
	state.Clients[c.ID] = c

	var lastOverlay modes.ClientOverlay

	m = &serverMutator{
		state:    state,
		queue:    command.NewQueue(),
		shutdown: func() {},
		pushOverlayFn: func(_ session.ClientID, ov modes.ClientOverlay) {
			lastOverlay = ov
		},
	}

	return m, func() modes.ClientOverlay { return lastOverlay }
}

// TestDisplayMenu_AttachesOverlay verifies that DisplayMenu pushes a
// *menu.Mode as a ClientOverlay onto the target client.
func TestDisplayMenu_AttachesOverlay(t *testing.T) {
	m, pushed := newTestMutatorWithDisplayMenu()

	items := []command.MenuEntry{
		{Label: "New Window", Key: "n", Command: "new-window"},
	}
	if err := m.DisplayMenu("c1", items); err != nil {
		t.Fatalf("DisplayMenu: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("pushOverlayFn not called or overlay is nil")
	}
	if _, ok := ov.(*menumode.Mode); !ok {
		t.Errorf("pushed overlay type = %T, want *menu.Mode", ov)
	}
}

// TestDisplayMenu_ClientNotFound verifies that an error is returned when the
// client does not exist.
func TestDisplayMenu_ClientNotFound(t *testing.T) {
	m, _ := newTestMutatorWithDisplayMenu()

	items := []command.MenuEntry{
		{Label: "New Window", Key: "n", Command: "new-window"},
	}
	if err := m.DisplayMenu("no-such-client", items); err == nil {
		t.Fatal("expected error for unknown client, got nil")
	}
}

// TestDisplayMenu_RendersItems verifies that the menu overlay renders all
// provided items and produces the correct number of cells.
func TestDisplayMenu_RendersItems(t *testing.T) {
	m, pushed := newTestMutatorWithDisplayMenu()

	items := []command.MenuEntry{
		{Label: "Alpha", Key: "a", Command: "new-window"},
		{Label: "Beta", Key: "b", Command: "new-session"},
	}
	if err := m.DisplayMenu("c1", items); err != nil {
		t.Fatalf("DisplayMenu: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("overlay is nil")
	}

	rect := ov.Rect()
	// Height must equal the number of items.
	if rect.Height != len(items) {
		t.Errorf("overlay height = %d, want %d", rect.Height, len(items))
	}
	// Width must be at least len("Alpha") + 2-char prefix = 7.
	if rect.Width < 7 {
		t.Errorf("overlay width = %d, want >= 7", rect.Width)
	}

	dst := make([]modes.Cell, rect.Width*rect.Height)
	ov.Render(dst) // must not panic
}

// TestDisplayMenu_SelectionEnqueuesCommand verifies that selecting an item
// enqueues its command string in the queue.
func TestDisplayMenu_SelectionEnqueuesCommand(t *testing.T) {
	m, pushed := newTestMutatorWithDisplayMenu()

	items := []command.MenuEntry{
		{Label: "New Window", Key: "n", Command: "new-window"},
	}
	if err := m.DisplayMenu("c1", items); err != nil {
		t.Fatalf("DisplayMenu: %v", err)
	}

	ov, ok := pushed().(*menumode.Mode)
	if !ok {
		t.Fatal("overlay is not *menu.Mode")
	}

	queueBefore := m.queue.Len()

	// Press Enter to activate the first (pre-selected) item.
	outcome := ov.Key(keys.Key{Code: keys.CodeEnter})
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("Enter outcome = %v, want KindCloseMode", outcome.Kind)
	}

	if got := m.queue.Len(); got <= queueBefore {
		t.Errorf("queue length after selection = %d, want > %d", got, queueBefore)
	}
}

// TestDisplayMenu_EscapeClosesWithoutAction verifies that pressing Escape
// closes the overlay without enqueuing any command.
func TestDisplayMenu_EscapeClosesWithoutAction(t *testing.T) {
	m, pushed := newTestMutatorWithDisplayMenu()

	items := []command.MenuEntry{
		{Label: "New Window", Key: "n", Command: "new-window"},
	}
	if err := m.DisplayMenu("c1", items); err != nil {
		t.Fatalf("DisplayMenu: %v", err)
	}

	ov, ok := pushed().(*menumode.Mode)
	if !ok {
		t.Fatal("overlay is not *menu.Mode")
	}

	queueBefore := m.queue.Len()

	// Press Escape to cancel.
	outcome := ov.Key(keys.Key{Code: keys.CodeEscape})
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("Escape outcome = %v, want KindCloseMode", outcome.Kind)
	}

	if got := m.queue.Len(); got != queueBefore {
		t.Errorf("queue length after Escape = %d, want %d (no new items)", got, queueBefore)
	}
}

// TestDisplayMenu_MnemonicActivatesItem verifies that pressing a mnemonic key
// activates the corresponding item and enqueues its command.
func TestDisplayMenu_MnemonicActivatesItem(t *testing.T) {
	m, pushed := newTestMutatorWithDisplayMenu()

	items := []command.MenuEntry{
		{Label: "Alpha", Key: "a", Command: "new-window"},
		{Label: "Beta", Key: "b", Command: "new-session"},
	}
	if err := m.DisplayMenu("c1", items); err != nil {
		t.Fatalf("DisplayMenu: %v", err)
	}

	ov, ok := pushed().(*menumode.Mode)
	if !ok {
		t.Fatal("overlay is not *menu.Mode")
	}

	queueBefore := m.queue.Len()

	// Press 'b' to activate "Beta" (new-session).
	outcome := ov.Key(keys.Key{Code: 'b'})
	if outcome.Kind != modes.KindCloseMode {
		t.Errorf("mnemonic 'b' outcome = %v, want KindCloseMode", outcome.Kind)
	}

	if got := m.queue.Len(); got <= queueBefore {
		t.Errorf("queue length after mnemonic = %d, want > %d", got, queueBefore)
	}
}

// TestDisplayMenu_SeparatorNotSelectable verifies that separator items (empty
// label/key/command) are rendered as separators and cannot be activated.
func TestDisplayMenu_SeparatorNotSelectable(t *testing.T) {
	m, pushed := newTestMutatorWithDisplayMenu()

	// Separator in the middle; the menu should skip it.
	items := []command.MenuEntry{
		{Label: "", Key: "", Command: ""},         // separator
		{Label: "New Window", Key: "n", Command: "new-window"},
	}
	if err := m.DisplayMenu("c1", items); err != nil {
		t.Fatalf("DisplayMenu: %v", err)
	}

	ov, ok := pushed().(*menumode.Mode)
	if !ok {
		t.Fatal("overlay is not *menu.Mode")
	}

	// The first selectable item is index 1 (separator is index 0).
	if ov.Selected() != 1 {
		t.Errorf("initial selected index = %d, want 1 (first non-separator)", ov.Selected())
	}
}

// TestDisplayMenu_EmptyItems pushes a menu with no items; should not panic.
func TestDisplayMenu_EmptyItems(t *testing.T) {
	m, pushed := newTestMutatorWithDisplayMenu()

	if err := m.DisplayMenu("c1", nil); err != nil {
		t.Fatalf("DisplayMenu: %v", err)
	}

	ov := pushed()
	if ov == nil {
		t.Fatal("overlay is nil")
	}

	rect := ov.Rect()
	dst := make([]modes.Cell, rect.Width*rect.Height)
	ov.Render(dst) // must not panic
}
