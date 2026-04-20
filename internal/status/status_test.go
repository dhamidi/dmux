package status_test

import (
	"strings"
	"testing"

	"github.com/dhamidi/dmux/internal/status"
)

// --- test doubles ---

// stubExpander replaces #{key} directives by looking up key in ctx.
// It does not support nested directives; it is sufficient for unit tests.
type stubExpander struct{}

func (e *stubExpander) Expand(template string, ctx status.Context) string {
	var out strings.Builder
	s := template
	for {
		start := strings.Index(s, "#{")
		if start < 0 {
			out.WriteString(s)
			break
		}
		out.WriteString(s[:start])
		s = s[start+2:]
		end := strings.IndexByte(s, '}')
		if end < 0 {
			out.WriteString("#{")
			out.WriteString(s)
			break
		}
		key := s[:end]
		s = s[end+1:]
		if v, ok := ctx.Lookup(key); ok {
			out.WriteString(v)
		}
	}
	return out.String()
}

// stubContext is a map-backed Context.
type stubContext map[string]string

func (m stubContext) Lookup(key string) (string, bool) {
	v, ok := m[key]
	return v, ok
}

// stubOptions holds canned format strings.
type stubOptions struct {
	left    string
	right   string
	formats []string
}

func (o *stubOptions) StatusLeft() string        { return o.left }
func (o *stubOptions) StatusRight() string       { return o.right }
func (o *stubOptions) StatusLineCount() int      { return len(o.formats) }
func (o *stubOptions) StatusFormat(n int) string {
	if n < len(o.formats) {
		return o.formats[n]
	}
	return ""
}

// cellsToString converts a []Cell to the string of their Char runes.
func cellsToString(cells []status.Cell) string {
	runes := make([]rune, len(cells))
	for i, c := range cells {
		if c.Char == 0 {
			runes[i] = ' '
		} else {
			runes[i] = c.Char
		}
	}
	return string(runes)
}

// --- tests ---

func TestRender_ExactWidth(t *testing.T) {
	exp := &stubExpander{}
	ctx := stubContext{"session_name": "main"}
	opts := &stubOptions{formats: []string{"#{session_name}"}}

	sl := status.New(exp, ctx, opts)
	cells := sl.Render(10)

	if len(cells) != 10 {
		t.Fatalf("want 10 cells, got %d", len(cells))
	}
	got := cellsToString(cells)
	want := "main      "
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestRender_TruncatesLongContent(t *testing.T) {
	exp := &stubExpander{}
	ctx := stubContext{"session_name": "a-very-long-session-name"}
	opts := &stubOptions{formats: []string{"#{session_name}"}}

	sl := status.New(exp, ctx, opts)
	cells := sl.Render(5)

	if len(cells) != 5 {
		t.Fatalf("want 5 cells, got %d", len(cells))
	}
	got := cellsToString(cells)
	want := "a-ver"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestRender_ZeroWidthReturnsNil(t *testing.T) {
	exp := &stubExpander{}
	ctx := stubContext{}
	opts := &stubOptions{formats: []string{"hello"}}

	sl := status.New(exp, ctx, opts)
	if sl.Render(0) != nil {
		t.Error("want nil for width 0")
	}
	if sl.Render(-1) != nil {
		t.Error("want nil for negative width")
	}
}

func TestRender_FallsBackToLeftRight(t *testing.T) {
	exp := &stubExpander{}
	ctx := stubContext{
		"session_name": "work",
		"date":         "Mon",
	}
	// No status-format-0 — use left + right fallback.
	opts := &stubOptions{
		left:    "#{session_name}",
		right:   "#{date}",
		formats: nil,
	}

	sl := status.New(exp, ctx, opts)
	cells := sl.Render(12)

	if len(cells) != 12 {
		t.Fatalf("want 12 cells, got %d", len(cells))
	}
	got := cellsToString(cells)
	want := "work Mon    "
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestRender_FallsBackToLeftOnly(t *testing.T) {
	exp := &stubExpander{}
	ctx := stubContext{"session_name": "proj"}
	opts := &stubOptions{left: "#{session_name}", formats: nil}

	sl := status.New(exp, ctx, opts)
	cells := sl.Render(8)

	got := cellsToString(cells)
	want := "proj    "
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestRender_StripsStyleMarkers(t *testing.T) {
	exp := &stubExpander{}
	ctx := stubContext{"session_name": "dev"}
	// Style markers should be stripped before computing cell width.
	opts := &stubOptions{formats: []string{"#[fg=green]#{session_name}#[default]"}}

	sl := status.New(exp, ctx, opts)
	cells := sl.Render(6)

	got := cellsToString(cells)
	want := "dev   "
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestRender_StripsMultipleStyleMarkers(t *testing.T) {
	exp := &stubExpander{}
	ctx := stubContext{"a": "X", "b": "Y"}
	opts := &stubOptions{formats: []string{"#[bold]#{a}#[nobold,fg=red]#{b}#[default]"}}

	sl := status.New(exp, ctx, opts)
	cells := sl.Render(4)

	got := cellsToString(cells)
	want := "XY  "
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestRender_EmptyOptions(t *testing.T) {
	exp := &stubExpander{}
	ctx := stubContext{}
	opts := &stubOptions{}

	sl := status.New(exp, ctx, opts)
	cells := sl.Render(5)

	got := cellsToString(cells)
	want := "     "
	if got != want {
		t.Errorf("want %q (all spaces), got %q", want, got)
	}
}

func TestLines_MultipleStatusLines(t *testing.T) {
	exp := &stubExpander{}
	ctx := stubContext{
		"session_name": "proj",
		"window_name":  "editor",
	}
	opts := &stubOptions{
		formats: []string{
			"#{session_name}",
			"#{window_name}",
		},
	}

	sl := status.New(exp, ctx, opts)
	lines := sl.Lines(10)

	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}
	if got := cellsToString(lines[0]); got != "proj      " {
		t.Errorf("line 0: want %q, got %q", "proj      ", got)
	}
	if got := cellsToString(lines[1]); got != "editor    " {
		t.Errorf("line 1: want %q, got %q", "editor    ", got)
	}
}

func TestLines_NoFormatsReturnsNil(t *testing.T) {
	exp := &stubExpander{}
	ctx := stubContext{}
	opts := &stubOptions{}

	sl := status.New(exp, ctx, opts)
	if sl.Lines(10) != nil {
		t.Error("want nil Lines when StatusLineCount is 0")
	}
}

func TestRender_LiteralTextNoVariables(t *testing.T) {
	exp := &stubExpander{}
	ctx := stubContext{}
	opts := &stubOptions{formats: []string{"hello"}}

	sl := status.New(exp, ctx, opts)
	cells := sl.Render(8)

	got := cellsToString(cells)
	want := "hello   "
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}
