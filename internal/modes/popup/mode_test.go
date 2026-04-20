package popup_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/modes"
	"github.com/dhamidi/dmux/internal/modes/popup"
	"github.com/dhamidi/dmux/internal/pane"
)

// --- fake pane --------------------------------------------------------------

// fakePane is an in-memory implementation of popup.Pane for tests.
// It records calls and allows setting the Snapshot return value.
type fakePane struct {
	mu          sync.Mutex
	written     []byte
	sentKeys    []keys.Key
	sentMouse   []keys.MouseEvent
	resizes     []resizeCall
	snapshot    pane.CellGrid
	closeCalled bool
}

type resizeCall struct{ cols, rows int }

func (f *fakePane) Write(data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.written = append(f.written, data...)
	return nil
}

func (f *fakePane) SendKey(k keys.Key) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentKeys = append(f.sentKeys, k)
	return nil
}

func (f *fakePane) SendMouse(ev keys.MouseEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentMouse = append(f.sentMouse, ev)
	return nil
}

func (f *fakePane) Resize(cols, rows int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resizes = append(f.resizes, resizeCall{cols, rows})
	return nil
}

func (f *fakePane) Snapshot() pane.CellGrid {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snapshot
}

func (f *fakePane) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeCalled = true
	return nil
}

func (f *fakePane) SentKeys() []keys.Key {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]keys.Key, len(f.sentKeys))
	copy(out, f.sentKeys)
	return out
}

func (f *fakePane) Closed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closeCalled
}

func (f *fakePane) Resizes() []resizeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]resizeCall, len(f.resizes))
	copy(out, f.resizes)
	return out
}

func (f *fakePane) SentMouse() []keys.MouseEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]keys.MouseEvent, len(f.sentMouse))
	copy(out, f.sentMouse)
	return out
}

// --- factory helpers --------------------------------------------------------

// newFakeFactory returns a PaneFactory that creates and returns fp.
func newFakeFactory(fp *fakePane) popup.PaneFactory {
	return func(rows, cols int, command string) (popup.Pane, error) {
		fp.mu.Lock()
		fp.resizes = append(fp.resizes, resizeCall{cols, rows})
		fp.mu.Unlock()
		return fp, nil
	}
}

// newErrorFactory returns a PaneFactory that always returns an error.
func newErrorFactory(err error) popup.PaneFactory {
	return func(rows, cols int, command string) (popup.Pane, error) {
		return nil, err
	}
}

// newPopup is a test helper that creates a popup with a fakePane.
// rect is the outer bounding box including the border.
func newPopup(t *testing.T, rect modes.Rect) (*popup.Mode, *fakePane) {
	t.Helper()
	fp := &fakePane{}
	m, err := popup.New(rect, "test", false, newFakeFactory(fp))
	if err != nil {
		t.Fatalf("popup.New: %v", err)
	}
	return m, fp
}

// renderDst allocates a []modes.Cell slice for a rect and calls Render.
func renderDst(m *popup.Mode) []modes.Cell {
	r := m.Rect()
	dst := make([]modes.Cell, r.Width*r.Height)
	m.Render(dst)
	return dst
}

// cellAt returns the character at (col, row) in dst given width w.
func cellAt(dst []modes.Cell, w, col, row int) rune {
	return dst[row*w+col].Char
}

// --- interface compliance ---------------------------------------------------

func TestInterfaceCompliance(t *testing.T) {
	var _ modes.ClientOverlay = (*popup.Mode)(nil)
}

// --- construction -----------------------------------------------------------

func TestNew_Rect(t *testing.T) {
	rect := modes.Rect{X: 5, Y: 3, Width: 20, Height: 10}
	m, _ := newPopup(t, rect)
	if got := m.Rect(); got != rect {
		t.Errorf("Rect() = %v, want %v", got, rect)
	}
}

func TestNew_PaneSizedToInner(t *testing.T) {
	// Outer rect 20×10 → inner 18×8.
	rect := modes.Rect{X: 0, Y: 0, Width: 20, Height: 10}
	fp := &fakePane{}
	_, err := popup.New(rect, "cmd", false, newFakeFactory(fp))
	if err != nil {
		t.Fatalf("popup.New: %v", err)
	}
	resizes := fp.Resizes()
	if len(resizes) != 1 {
		t.Fatalf("expected 1 resize call, got %d", len(resizes))
	}
	// factory is called with (rows, cols, command) and fakeFactory stores (cols, rows).
	wantCols := rect.Width - 2   // 18
	wantRows := rect.Height - 2  // 8
	if resizes[0].cols != wantCols {
		t.Errorf("pane cols = %d, want %d", resizes[0].cols, wantCols)
	}
	if resizes[0].rows != wantRows {
		t.Errorf("pane rows = %d, want %d", resizes[0].rows, wantRows)
	}
}

func TestNew_FactoryError(t *testing.T) {
	rect := modes.Rect{X: 0, Y: 0, Width: 10, Height: 6}
	sentinel := errors.New("pty failed")
	_, err := popup.New(rect, "cmd", false, newErrorFactory(sentinel))
	if err == nil {
		t.Fatal("expected error from factory, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want wrapping %v", err, sentinel)
	}
}

// --- rendering --------------------------------------------------------------

func TestRender_Border(t *testing.T) {
	rect := modes.Rect{X: 0, Y: 0, Width: 6, Height: 4}
	m, _ := newPopup(t, rect)
	dst := renderDst(m)
	w := rect.Width

	// Corners.
	tests := []struct {
		col, row int
		want     rune
	}{
		{0, 0, '┌'}, {w - 1, 0, '┐'},
		{0, rect.Height - 1, '└'}, {w - 1, rect.Height - 1, '┘'},
	}
	for _, tc := range tests {
		if got := cellAt(dst, w, tc.col, tc.row); got != tc.want {
			t.Errorf("cell(%d,%d) = %q, want %q", tc.col, tc.row, got, tc.want)
		}
	}

	// Top and bottom horizontal edges.
	for col := 1; col < w-1; col++ {
		if got := cellAt(dst, w, col, 0); got != '─' {
			t.Errorf("top edge col=%d = %q, want '─'", col, got)
		}
		if got := cellAt(dst, w, col, rect.Height-1); got != '─' {
			t.Errorf("bottom edge col=%d = %q, want '─'", col, got)
		}
	}

	// Left and right vertical edges.
	for row := 1; row < rect.Height-1; row++ {
		if got := cellAt(dst, w, 0, row); got != '│' {
			t.Errorf("left edge row=%d = %q, want '│'", row, got)
		}
		if got := cellAt(dst, w, w-1, row); got != '│' {
			t.Errorf("right edge row=%d = %q, want '│'", row, got)
		}
	}
}

func TestRender_PaneContent(t *testing.T) {
	rect := modes.Rect{X: 0, Y: 0, Width: 6, Height: 4}
	fp := &fakePane{}
	// Set up a 4×2 grid (inner area for a 6×4 popup) with 'A' everywhere.
	innerCols := rect.Width - 2  // 4
	innerRows := rect.Height - 2 // 2
	cells := make([]pane.Cell, innerCols*innerRows)
	for i := range cells {
		cells[i] = pane.Cell{Char: 'A'}
	}
	fp.snapshot = pane.CellGrid{Rows: innerRows, Cols: innerCols, Cells: cells}
	m, err := popup.New(rect, "cmd", false, newFakeFactory(fp))
	if err != nil {
		t.Fatalf("popup.New: %v", err)
	}
	dst := renderDst(m)
	w := rect.Width

	// Inner cells (row 1..2, col 1..4) should all be 'A'.
	for row := 1; row < rect.Height-1; row++ {
		for col := 1; col < rect.Width-1; col++ {
			if got := cellAt(dst, w, col, row); got != 'A' {
				t.Errorf("inner cell(%d,%d) = %q, want 'A'", col, row, got)
			}
		}
	}
}

func TestRender_EmptyPane(t *testing.T) {
	// Empty snapshot (zeroed cells) → interior should be spaces.
	rect := modes.Rect{X: 0, Y: 0, Width: 5, Height: 4}
	m, _ := newPopup(t, rect)
	dst := renderDst(m)
	w := rect.Width

	for row := 1; row < rect.Height-1; row++ {
		for col := 1; col < rect.Width-1; col++ {
			if got := cellAt(dst, w, col, row); got != ' ' {
				t.Errorf("inner cell(%d,%d) = %q, want ' '", col, row, got)
			}
		}
	}
}

// --- key forwarding ---------------------------------------------------------

func TestKey_Escape_ClosesPopup(t *testing.T) {
	m, _ := newPopup(t, modes.Rect{X: 0, Y: 0, Width: 10, Height: 6})
	out := m.Key(keys.Key{Code: keys.CodeEscape})
	if out.Kind != modes.KindCloseMode {
		t.Errorf("Escape outcome = %v, want KindCloseMode", out.Kind)
	}
}

func TestKey_ForwardedToPane(t *testing.T) {
	m, fp := newPopup(t, modes.Rect{X: 0, Y: 0, Width: 10, Height: 6})
	k := keys.Key{Code: keys.KeyCode('a')}
	out := m.Key(k)
	if out.Kind != modes.KindConsumed {
		t.Errorf("Key outcome = %v, want KindConsumed", out.Kind)
	}
	if sent := fp.SentKeys(); len(sent) != 1 || sent[0] != k {
		t.Errorf("pane.SentKeys() = %v, want [%v]", sent, k)
	}
}

func TestKey_EscapeNotForwarded(t *testing.T) {
	m, fp := newPopup(t, modes.Rect{X: 0, Y: 0, Width: 10, Height: 6})
	m.Key(keys.Key{Code: keys.CodeEscape})
	if sent := fp.SentKeys(); len(sent) != 0 {
		t.Errorf("Escape should not be forwarded to pane, got %v", sent)
	}
}

// --- mouse forwarding -------------------------------------------------------

func TestMouse_InsideForwardedToPane(t *testing.T) {
	rect := modes.Rect{X: 2, Y: 3, Width: 10, Height: 6}
	m, fp := newPopup(t, rect)
	ev := keys.MouseEvent{
		Action: keys.MousePress,
		Button: keys.MouseLeft,
		Col:    5, // inside rect
		Row:    5,
	}
	out := m.Mouse(ev)
	if out.Kind != modes.KindConsumed {
		t.Errorf("Mouse inside outcome = %v, want KindConsumed", out.Kind)
	}
	if sent := fp.SentMouse(); len(sent) != 1 || sent[0] != ev {
		t.Errorf("pane.SentMouse() = %v, want [%v]", sent, ev)
	}
}

func TestMouse_OutsidePassthrough(t *testing.T) {
	rect := modes.Rect{X: 2, Y: 3, Width: 10, Height: 6}
	m, fp := newPopup(t, rect)
	ev := keys.MouseEvent{
		Action: keys.MousePress,
		Button: keys.MouseLeft,
		Col:    0, // outside rect (rect.X == 2)
		Row:    0,
	}
	out := m.Mouse(ev)
	if out.Kind != modes.KindPassthrough {
		t.Errorf("Mouse outside outcome = %v, want KindPassthrough", out.Kind)
	}
	if sent := fp.SentMouse(); len(sent) != 0 {
		t.Errorf("outside mouse should not reach pane, got %v", sent)
	}
}

// --- focus and close --------------------------------------------------------

func TestCaptureFocus(t *testing.T) {
	m, _ := newPopup(t, modes.Rect{X: 0, Y: 0, Width: 10, Height: 6})
	if !m.CaptureFocus() {
		t.Error("CaptureFocus() = false, want true")
	}
}

func TestClose_KillsPane(t *testing.T) {
	m, fp := newPopup(t, modes.Rect{X: 0, Y: 0, Width: 10, Height: 6})
	m.Close()
	if !fp.Closed() {
		t.Error("Close() did not close the underlying pane")
	}
}

func TestClose_Idempotent(t *testing.T) {
	m, _ := newPopup(t, modes.Rect{X: 0, Y: 0, Width: 10, Height: 6})
	m.Close()
	m.Close() // must not panic
}
