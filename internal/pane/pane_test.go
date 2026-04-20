package pane_test

import (
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/pty"
)

// newTestPane builds a Pane backed by a FakePTY and fake encoders/terminal.
// The pane is closed automatically via t.Cleanup.
func newTestPane(t *testing.T, id pane.PaneID) (pane.Pane, *pty.FakePTY, *pane.FakeTerminal, *pane.FakeKeyEncoder, *pane.FakeMouseEncoder) {
	t.Helper()
	fp := &pty.FakePTY{}
	ft := &pane.FakeTerminal{}
	fk := &pane.FakeKeyEncoder{}
	fm := &pane.FakeMouseEncoder{}
	p, err := pane.New(pane.Config{
		ID:    id,
		PTY:   fp,
		Term:  ft,
		Keys:  fk,
		Mouse: fm,
	})
	if err != nil {
		t.Fatalf("pane.New: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p, fp, ft, fk, fm
}

func TestPane_ID(t *testing.T) {
	p, _, _, _, _ := newTestPane(t, 42)
	if p.ID() != 42 {
		t.Fatalf("ID() = %d, want 42", p.ID())
	}
}

func TestPane_Title_DelegatesToTerminal(t *testing.T) {
	p, _, ft, _, _ := newTestPane(t, 1)
	ft.SetTitle("zsh")
	if got := p.Title(); got != "zsh" {
		t.Fatalf("Title() = %q, want %q", got, "zsh")
	}
}

func TestPane_Write_RoutesToPTY(t *testing.T) {
	p, fp, _, _, _ := newTestPane(t, 1)
	data := []byte("hello")
	if err := p.Write(data); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := string(fp.Input()); got != "hello" {
		t.Fatalf("PTY input = %q, want %q", got, "hello")
	}
}

func TestPane_SendKey_EncodesAndRoutes(t *testing.T) {
	p, fp, _, fk, _ := newTestPane(t, 1)
	key, err := keys.Parse("C-a")
	if err != nil {
		t.Fatalf("keys.Parse: %v", err)
	}
	if err := p.SendKey(key); err != nil {
		t.Fatalf("SendKey: %v", err)
	}
	// Verify the key was recorded by the encoder.
	encoded := fk.Encoded()
	if len(encoded) != 1 || encoded[0] != key {
		t.Fatalf("encoded keys = %v, want [%v]", encoded, key)
	}
	// Verify encoded bytes reached the PTY.
	if got := fp.Input(); len(got) == 0 {
		t.Fatal("PTY received no input after SendKey")
	}
}

func TestPane_SendMouse_EncodesAndRoutes(t *testing.T) {
	p, fp, _, _, fm := newTestPane(t, 1)
	ev := keys.MouseEvent{
		Action: keys.MousePress,
		Button: keys.MouseLeft,
		Col:    5,
		Row:    3,
	}
	if err := p.SendMouse(ev); err != nil {
		t.Fatalf("SendMouse: %v", err)
	}
	// Verify the event was recorded by the encoder.
	encoded := fm.Encoded()
	if len(encoded) != 1 {
		t.Fatalf("encoded mouse events = %d, want 1", len(encoded))
	}
	// Verify encoded bytes reached the PTY.
	if got := fp.Input(); len(got) == 0 {
		t.Fatal("PTY received no input after SendMouse")
	}
}

func TestPane_Resize_UpdatesBothPTYAndTerminal(t *testing.T) {
	p, fp, ft, _, _ := newTestPane(t, 1)
	if err := p.Resize(80, 24); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	// PTY resize: pty.Resize receives (rows, cols).
	if len(fp.Resizes) != 1 {
		t.Fatalf("PTY resize count = %d, want 1", len(fp.Resizes))
	}
	if fp.Resizes[0].Cols != 80 || fp.Resizes[0].Rows != 24 {
		t.Fatalf("PTY resize = %+v, want {Cols:80, Rows:24}", fp.Resizes[0])
	}
	// Terminal resize: term.Resize receives (cols, rows).
	calls := ft.ResizeCalls()
	if len(calls) != 1 {
		t.Fatalf("terminal resize count = %d, want 1", len(calls))
	}
	if calls[0].Cols != 80 || calls[0].Rows != 24 {
		t.Fatalf("terminal resize = %+v, want {Cols:80, Rows:24}", calls[0])
	}
}

func TestPane_CopyOutput_FeedsTerminal(t *testing.T) {
	fp := &pty.FakePTY{}
	ft := &pane.FakeTerminal{}
	output := []byte("\x1b[32mhello\x1b[0m")
	fp.InjectOutput(output)

	p, err := pane.New(pane.Config{
		ID:    1,
		PTY:   fp,
		Term:  ft,
		Keys:  &pane.FakeKeyEncoder{},
		Mouse: &pane.FakeMouseEncoder{},
	})
	if err != nil {
		t.Fatalf("pane.New: %v", err)
	}
	defer p.Close()

	// Poll until the goroutine copies the output (or deadline).
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(ft.Written()) >= len(output) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := ft.Written(); string(got) != string(output) {
		t.Fatalf("terminal received %q, want %q", got, output)
	}
}

func TestPane_Snapshot_DelegatesToTerminal(t *testing.T) {
	p, _, ft, _, _ := newTestPane(t, 1)
	grid := pane.CellGrid{
		Rows:  24,
		Cols:  80,
		Cells: make([]pane.Cell, 24*80),
	}
	grid.Cells[0] = pane.Cell{Char: 'A'}
	ft.SetGrid(grid)

	got := p.Snapshot()
	if got.Rows != 24 || got.Cols != 80 {
		t.Fatalf("Snapshot dims = (%d, %d), want (24, 80)", got.Rows, got.Cols)
	}
	if got.Cells[0].Char != 'A' {
		t.Fatalf("Snapshot().Cells[0].Char = %q, want 'A'", got.Cells[0].Char)
	}
}

func TestPane_Close_ReleasesEncoders(t *testing.T) {
	fp := &pty.FakePTY{}
	fk := &pane.FakeKeyEncoder{}
	fm := &pane.FakeMouseEncoder{}
	p, err := pane.New(pane.Config{
		ID:    1,
		PTY:   fp,
		Term:  &pane.FakeTerminal{},
		Keys:  fk,
		Mouse: fm,
	})
	if err != nil {
		t.Fatalf("pane.New: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !fk.Closed() {
		t.Error("key encoder not closed after pane.Close")
	}
	if !fm.Closed() {
		t.Error("mouse encoder not closed after pane.Close")
	}
}

func TestPane_CaptureContent_RendersGrid(t *testing.T) {
	p, _, ft, _, _ := newTestPane(t, 1)
	grid := pane.CellGrid{
		Rows:  2,
		Cols:  3,
		Cells: make([]pane.Cell, 2*3),
	}
	// First row: "Hi "
	grid.Cells[0] = pane.Cell{Char: 'H'}
	grid.Cells[1] = pane.Cell{Char: 'i'}
	// grid.Cells[2] is zero → space
	// Second row: "ok "
	grid.Cells[3] = pane.Cell{Char: 'o'}
	grid.Cells[4] = pane.Cell{Char: 'k'}
	ft.SetGrid(grid)

	content, err := p.CaptureContent(false)
	if err != nil {
		t.Fatalf("CaptureContent: %v", err)
	}
	want := "Hi \nok \n"
	if string(content) != want {
		t.Fatalf("CaptureContent = %q, want %q", content, want)
	}
}

func TestPane_CaptureContent_EmptyGrid(t *testing.T) {
	p, _, _, _, _ := newTestPane(t, 1)
	content, err := p.CaptureContent(false)
	if err != nil {
		t.Fatalf("CaptureContent: %v", err)
	}
	if len(content) != 0 {
		t.Fatalf("CaptureContent on empty grid = %q, want empty", content)
	}
}

func TestPane_Respawn_ClosesOldPTYAndOpensNew(t *testing.T) {
	oldPTY := &pty.FakePTY{}
	newPTY := &pty.FakePTY{}

	ft := &pane.FakeTerminal{}
	fk := &pane.FakeKeyEncoder{}
	fm := &pane.FakeMouseEncoder{}

	factory := func(shell string, cols, rows int) (pty.PTY, error) {
		return newPTY, nil
	}

	p, err := pane.New(pane.Config{
		ID:         1,
		PTY:        oldPTY,
		Term:       ft,
		Keys:       fk,
		Mouse:      fm,
		PTYFactory: factory,
	})
	if err != nil {
		t.Fatalf("pane.New: %v", err)
	}
	defer p.Close()

	if err := p.Respawn("bash"); err != nil {
		t.Fatalf("Respawn: %v", err)
	}

	// The old PTY should have been closed by Respawn.
	// We verify by checking that the old FakePTY was closed.
	// FakePTY.Close sets closed=true; subsequent Writes return ErrClosedPipe.
	if _, err := oldPTY.Write([]byte("test")); err == nil {
		t.Error("expected old PTY to be closed after Respawn, but Write succeeded")
	}
}

func TestPane_Respawn_ErrorWithoutFactory(t *testing.T) {
	p, _, _, _, _ := newTestPane(t, 1)
	if err := p.Respawn(""); err == nil {
		t.Error("expected error from Respawn with no PTYFactory, got nil")
	}
}

func TestPane_Close_ReleasesTerminal(t *testing.T) {
	fp := &pty.FakePTY{}
	ft := &pane.FakeTerminal{}
	p, err := pane.New(pane.Config{
		ID:    1,
		PTY:   fp,
		Term:  ft,
		Keys:  &pane.FakeKeyEncoder{},
		Mouse: &pane.FakeMouseEncoder{},
	})
	if err != nil {
		t.Fatalf("pane.New: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !ft.Closed() {
		t.Error("terminal not closed after pane.Close")
	}
}
