package vt

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// newRuntime is the common "open a Runtime, schedule Close" helper
// used by every test below. Failures here end the test immediately
// because every vt test depends on a working runtime.
func newRuntime(t *testing.T) *Runtime {
	t.Helper()
	ctx := context.Background()
	r, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = r.Close(ctx) })
	return r
}

func newTerminal(t *testing.T, r *Runtime, cols, rows int) *Terminal {
	t.Helper()
	ctx := context.Background()
	term, err := r.NewTerminal(ctx, cols, rows)
	if err != nil {
		t.Fatalf("NewTerminal: %v", err)
	}
	t.Cleanup(func() { _ = term.Close() })
	return term
}

// snapshotText flattens a Grid to a multi-line string. Empty cells
// (codepoint 0) become spaces; spacer-tail cells are dropped so that
// a wide character appears as exactly one rune at its left column.
func snapshotText(g Grid) string {
	var b strings.Builder
	for y := 0; y < g.Rows; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		row := g.Cells[y]
		// Trim trailing empty cells so the string is comparable.
		end := len(row)
		for end > 0 && (row[end-1].Rune == 0 || row[end-1].Rune == ' ') {
			end--
		}
		for x := 0; x < end; x++ {
			c := row[x]
			if c.Wide == CellSpacerTail {
				continue
			}
			if c.Rune == 0 {
				b.WriteByte(' ')
			} else {
				b.WriteRune(c.Rune)
			}
		}
	}
	return b.String()
}

func TestNewTerminalDimensions(t *testing.T) {
	r := newRuntime(t)
	term := newTerminal(t, r, 80, 24)

	grid, err := term.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if grid.Rows != 24 {
		t.Errorf("Rows = %d, want 24", grid.Rows)
	}
	if grid.Cols != 80 {
		t.Errorf("Cols = %d, want 80", grid.Cols)
	}

	cur, err := term.Cursor()
	if err != nil {
		t.Fatalf("Cursor: %v", err)
	}
	if cur.X != 0 || cur.Y != 0 {
		t.Errorf("initial cursor = (%d,%d), want (0,0)", cur.X, cur.Y)
	}
	if !cur.Visible {
		t.Errorf("initial cursor should be visible")
	}
}

func TestFeedHelloWorld(t *testing.T) {
	r := newRuntime(t)
	term := newTerminal(t, r, 40, 5)

	if err := term.Feed([]byte("hello, world")); err != nil {
		t.Fatalf("Feed: %v", err)
	}

	grid, err := term.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	text := snapshotText(grid)
	if !strings.HasPrefix(text, "hello, world") {
		t.Errorf("snapshot first line = %q, want prefix %q", firstLine(text), "hello, world")
	}

	cur, err := term.Cursor()
	if err != nil {
		t.Fatalf("Cursor: %v", err)
	}
	if cur.X != 12 || cur.Y != 0 {
		t.Errorf("cursor after Feed = (%d,%d), want (12,0)", cur.X, cur.Y)
	}
}

func TestFeedNewlineAdvancesCursor(t *testing.T) {
	r := newRuntime(t)
	term := newTerminal(t, r, 20, 5)

	// "a\r\nb" — CR+LF so the cursor goes to (1, 1).
	if err := term.Feed([]byte("a\r\nb")); err != nil {
		t.Fatalf("Feed: %v", err)
	}

	grid, err := term.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if grid.Rows < 2 {
		t.Fatalf("grid too small: %d rows", grid.Rows)
	}
	if grid.Cells[0][0].Rune != 'a' {
		t.Errorf("[0][0] = %q, want 'a'", grid.Cells[0][0].Rune)
	}
	if grid.Cells[1][0].Rune != 'b' {
		t.Errorf("[1][0] = %q, want 'b'", grid.Cells[1][0].Rune)
	}

	cur, err := term.Cursor()
	if err != nil {
		t.Fatalf("Cursor: %v", err)
	}
	if cur.X != 1 || cur.Y != 1 {
		t.Errorf("cursor = (%d,%d), want (1,1)", cur.X, cur.Y)
	}
}

func TestResizeChangesGrid(t *testing.T) {
	r := newRuntime(t)
	term := newTerminal(t, r, 20, 5)

	if err := term.Resize(30, 10); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	grid, err := term.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if grid.Cols != 30 {
		t.Errorf("Cols after Resize = %d, want 30", grid.Cols)
	}
	if grid.Rows != 10 {
		t.Errorf("Rows after Resize = %d, want 10", grid.Rows)
	}
}

func TestCloseIsIdempotentAndReturnsErrClosed(t *testing.T) {
	r := newRuntime(t)
	term := newTerminal(t, r, 10, 3)

	if err := term.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := term.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := term.Feed([]byte("x")); !errors.Is(err, ErrClosed) {
		t.Errorf("Feed after Close: expected ErrClosed, got %v", err)
	}
	if err := term.Resize(5, 5); !errors.Is(err, ErrClosed) {
		t.Errorf("Resize after Close: expected ErrClosed, got %v", err)
	}
	if _, err := term.Snapshot(); !errors.Is(err, ErrClosed) {
		t.Errorf("Snapshot after Close: expected ErrClosed, got %v", err)
	}
	if _, err := term.Cursor(); !errors.Is(err, ErrClosed) {
		t.Errorf("Cursor after Close: expected ErrClosed, got %v", err)
	}
}

func TestMultipleTerminalsAreIndependent(t *testing.T) {
	r := newRuntime(t)
	a := newTerminal(t, r, 20, 5)
	b := newTerminal(t, r, 20, 5)

	if err := a.Feed([]byte("alpha")); err != nil {
		t.Fatalf("a.Feed: %v", err)
	}
	if err := b.Feed([]byte("bravo")); err != nil {
		t.Fatalf("b.Feed: %v", err)
	}

	ga, err := a.Snapshot()
	if err != nil {
		t.Fatalf("a.Snapshot: %v", err)
	}
	gb, err := b.Snapshot()
	if err != nil {
		t.Fatalf("b.Snapshot: %v", err)
	}
	if got := firstLine(snapshotText(ga)); !strings.HasPrefix(got, "alpha") {
		t.Errorf("a first line = %q, want prefix alpha", got)
	}
	if got := firstLine(snapshotText(gb)); !strings.HasPrefix(got, "bravo") {
		t.Errorf("b first line = %q, want prefix bravo", got)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
