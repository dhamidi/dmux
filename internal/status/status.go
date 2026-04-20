package status

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/dhamidi/dmux/internal/options"
	"github.com/dhamidi/dmux/internal/style"
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
	// StatusStyle returns the style string for the entire status line.
	StatusStyle() string
	// StatusLeftStyle returns the style string for the left-hand segment.
	StatusLeftStyle() string
	// StatusRightStyle returns the style string for the right-hand segment.
	StatusRightStyle() string
	// WindowStatusStyle returns the style string for inactive window entries.
	WindowStatusStyle() string
	// WindowStatusCurrentStyle returns the style string for the current window entry.
	WindowStatusCurrentStyle() string
}

// StoreOptions wraps an *options.Store and implements the Options interface
// by reading option values from the store.
type StoreOptions struct {
	Store *options.Store
}

func (o *StoreOptions) StatusLeft() string {
	v, _ := o.Store.GetString("status-left")
	return v
}

func (o *StoreOptions) StatusRight() string {
	v, _ := o.Store.GetString("status-right")
	return v
}

func (o *StoreOptions) StatusFormat(n int) string {
	key := fmt.Sprintf("status-format-%d", n)
	v, _ := o.Store.GetString(key)
	return v
}

func (o *StoreOptions) StatusLineCount() int {
	count := 0
	for i := 0; ; i++ {
		key := fmt.Sprintf("status-format-%d", i)
		if _, ok := o.Store.GetString(key); !ok {
			break
		}
		count++
	}
	return count
}

func (o *StoreOptions) StatusStyle() string {
	v, _ := o.Store.GetStyle("status-style")
	return v
}

func (o *StoreOptions) StatusLeftStyle() string {
	v, _ := o.Store.GetStyle("status-left-style")
	return v
}

func (o *StoreOptions) StatusRightStyle() string {
	v, _ := o.Store.GetStyle("status-right-style")
	return v
}

func (o *StoreOptions) WindowStatusStyle() string {
	v, _ := o.Store.GetStyle("window-status-style")
	return v
}

func (o *StoreOptions) WindowStatusCurrentStyle() string {
	v, _ := o.Store.GetStyle("window-status-current-style")
	return v
}

// Cell is a single display cell in a status line.
// Style carries the parsed #[...] attributes that were in effect at this
// cell position; callers may use it to drive ANSI rendering.
type Cell struct {
	Char  rune        // displayed character; 0 is treated as space by callers
	Style style.Style // active style at this cell position
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
// Style markers of the form #[...] are parsed; the resulting style is stored
// on each Cell so that callers can drive colour/attribute rendering.
// Cells beyond the length of the visible text are filled with spaces using
// the style that was in effect at end-of-text.
func renderLine(s string, width int) []Cell {
	cells := make([]Cell, width)
	col := 0
	var current style.Style
	var stack []style.Style

	i := 0
	for i < len(s) {
		// Detect #[ style markers.
		if i+1 < len(s) && s[i] == '#' && s[i+1] == '[' {
			end := strings.IndexByte(s[i+2:], ']')
			if end >= 0 {
				marker := s[i+2 : i+2+end]
				delta := style.Parse(marker)
				switch {
				case delta.Push:
					stack = append(stack, current)
				case delta.Pop:
					if len(stack) > 0 {
						current = stack[len(stack)-1]
						stack = stack[:len(stack)-1]
					}
				default:
					current = style.Apply(current, delta)
				}
				i = i + 2 + end + 1
				continue
			}
		}
		if col >= width {
			// Consume remaining input to handle trailing markers.
			r, size := utf8.DecodeRuneInString(s[i:])
			_ = r
			i += size
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		cells[col] = Cell{Char: r, Style: current}
		col++
		i += size
	}
	// Fill remaining cells with spaces using the last active style.
	for ; col < width; col++ {
		cells[col] = Cell{Char: ' ', Style: current}
	}
	return cells
}
