package status

import (
	"strings"
	"unicode/utf8"
)

// Context provides variable resolution for format string expansion.
// Implementations may be backed by a plain map[string]string or by live
// session state; the status package has no dependency on either.
// The format.MapContext type satisfies this interface directly via its
// Lookup method.
type Context interface {
	// Lookup returns the value of the named variable and true if the
	// variable exists. It returns ("", false) when the variable is not
	// defined.
	Lookup(key string) (string, bool)
}

// Expander expands #{...} format template strings against a Context.
// The concrete format.Expander satisfies this interface via an adapter
// that wraps the Context with a no-op Children method.
type Expander interface {
	// Expand substitutes #{...} directives in template using ctx for
	// variable lookups and returns the result. Expansion errors are
	// swallowed; an erroneous directive expands to an empty string.
	Expand(template string, ctx Context) string
}

// Options provides the format strings used to configure status lines.
// The concrete session.Options (via an adapter over options.Store)
// satisfies this interface.
type Options interface {
	// StatusLeft returns the format string for the left-hand segment.
	StatusLeft() string
	// StatusRight returns the format string for the right-hand segment.
	StatusRight() string
	// StatusFormat returns the format string for status line n (0-based).
	// Returns an empty string when line n is not configured.
	StatusFormat(n int) string
	// StatusLineCount returns the total number of configured status lines.
	StatusLineCount() int
}

// Cell is a single display cell in a status line.
type Cell struct {
	Char rune // displayed character; 0 is treated as space by callers
}

// Line is a horizontal slice of cells representing one row of the status
// bar. A Line produced by this package is always exactly the requested
// width in length.
type Line []Cell

// StatusLine renders one or more status lines from format strings.
// Create one with [New].
type StatusLine struct {
	expander Expander
	ctx      Context
	opts     Options
}

// New creates a StatusLine that expands format strings obtained from
// opts against ctx using expander.
func New(expander Expander, ctx Context, opts Options) *StatusLine {
	return &StatusLine{expander: expander, ctx: ctx, opts: opts}
}

// Render produces exactly width cells for the primary status line
// (status-format-0). When no format is configured, Render falls back to
// joining StatusLeft() and StatusRight() with a space.
// Returns nil when width ≤ 0.
//
// Render satisfies the render.StatusLine interface.
func (s *StatusLine) Render(width int) []Cell {
	if width <= 0 {
		return nil
	}
	tmpl := s.opts.StatusFormat(0)
	if tmpl == "" {
		left := s.opts.StatusLeft()
		right := s.opts.StatusRight()
		switch {
		case left == "" && right == "":
			// nothing configured
		case left == "":
			tmpl = right
		case right == "":
			tmpl = left
		default:
			tmpl = left + " " + right
		}
	}
	expanded := s.expander.Expand(tmpl, s.ctx)
	return renderLine(expanded, width)
}

// Lines produces one Line per configured status format string, expanding
// each against ctx. Returns nil when StatusLineCount() returns 0.
func (s *StatusLine) Lines(width int) []Line {
	n := s.opts.StatusLineCount()
	if n == 0 {
		return nil
	}
	lines := make([]Line, n)
	for i := range lines {
		tmpl := s.opts.StatusFormat(i)
		expanded := s.expander.Expand(tmpl, s.ctx)
		lines[i] = renderLine(expanded, width)
	}
	return lines
}

// renderLine converts an expanded string into a []Cell of exactly width cells.
// Style markers of the form #[...] are stripped. Cells beyond the length of
// the visible text are filled with spaces.
func renderLine(s string, width int) []Cell {
	s = stripStyleMarkers(s)
	cells := make([]Cell, width)
	col := 0
	for _, r := range s {
		if col >= width {
			break
		}
		cells[col] = Cell{Char: r}
		col++
	}
	for ; col < width; col++ {
		cells[col] = Cell{Char: ' '}
	}
	return cells
}

// stripStyleMarkers removes tmux-style #[attr,attr,...] sequences from s.
// Unmatched #[ sequences (no closing ]) are left in place.
func stripStyleMarkers(s string) string {
	if !strings.Contains(s, "#[") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '#' && s[i+1] == '[' {
			end := strings.IndexByte(s[i+2:], ']')
			if end >= 0 {
				i = i + 2 + end + 1
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		b.WriteRune(r)
		i += size
	}
	return b.String()
}
