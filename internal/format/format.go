package format

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Context provides variable resolution for format string expansion.
// Implementations may be backed by a plain map[string]string or by live
// session/pane state; the format package has no dependency on either.
type Context interface {
	// Lookup returns the value of the named variable and true if the variable
	// exists. It returns ("", false) when the variable is not defined.
	Lookup(key string) (string, bool)

	// Children returns sub-contexts for list variables (e.g. panes within a
	// window). Returns nil when listKey has no children.
	Children(listKey string) []Context
}

// CommandRunner executes external commands during format string expansion.
// The implementation is supplied by the caller; the format package never
// spawns processes directly.
type CommandRunner interface {
	// Run executes the program identified by name with the given arguments
	// and returns its combined output. An error is returned only when the
	// command cannot be started; non-zero exit codes are not errors.
	Run(name string, args []string) (output string, err error)
}

// MapContext is a [Context] backed by a plain map[string]string.
// It is intended for tests and standalone templating use cases where no
// session or pane state is available.
type MapContext map[string]string

// Lookup returns the value for key and true if the key exists in the map.
func (m MapContext) Lookup(key string) (string, bool) {
	v, ok := m[key]
	return v, ok
}

// Children always returns nil because MapContext has no nested structure.
func (m MapContext) Children(_ string) []Context {
	return nil
}

// Expander expands #{...} format strings using a [CommandRunner] for shell
// command execution. Create one with [New].
type Expander struct {
	runner CommandRunner
}

// New creates an [Expander] that uses runner to evaluate #(...) shell command
// directives. Passing nil disables shell command execution; any #(...) directive
// will expand to an empty string.
func New(runner CommandRunner) *Expander {
	return &Expander{runner: runner}
}

// Expand expands template using ctx for variable lookup with no shell command
// support. It is a convenience wrapper around New(nil).Expand(template, ctx).
func Expand(template string, ctx Context) (string, error) {
	return New(nil).Expand(template, ctx)
}

// Expand expands template using ctx for variable lookup and e's [CommandRunner]
// for #(...) shell command directives.
func (e *Expander) Expand(template string, ctx Context) (string, error) {
	return e.expand(template, ctx)
}

func (e *Expander) expand(s string, ctx Context) (string, error) {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] != '#' || i+1 >= len(s) {
			out.WriteByte(s[i])
			i++
			continue
		}
		switch s[i+1] {
		case '{':
			end := findMatchingBrace(s, i+2)
			if end < 0 {
				return "", fmt.Errorf("format: unclosed #{ at position %d", i)
			}
			inner := s[i+2 : end]
			val, err := e.evalDirective(inner, ctx)
			if err != nil {
				return "", err
			}
			out.WriteString(val)
			i = end + 1
		case '(':
			rest := s[i+2:]
			end := strings.Index(rest, ")")
			if end < 0 {
				return "", fmt.Errorf("format: unclosed #( at position %d", i)
			}
			cmd := rest[:end]
			out.WriteString(e.runShellCommand(cmd))
			i = i + 2 + end + 1
		case '[':
			// #[...] style marker — re-emit verbatim so that callers
			// (e.g. status.renderLine) can parse it for colour/attribute
			// information.  An unmatched #[ (no closing ]) is left in
			// place unchanged.
			rest := s[i+2:]
			end := strings.IndexByte(rest, ']')
			if end < 0 {
				// No closing bracket; emit the '#' and advance past it.
				out.WriteByte(s[i])
				i++
			} else {
				// Re-emit the complete #[...] sentinel.
				out.WriteString(s[i : i+2+end+1])
				i = i + 2 + end + 1
			}
		default:
			out.WriteByte(s[i])
			i++
		}
	}
	return out.String(), nil
}

// findMatchingBrace returns the index of the '}' that closes the '{' implied
// at position start (i.e. depth starts at 1). Returns -1 if not found.
func findMatchingBrace(s string, start int) int {
	depth := 1
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// evalDirective dispatches on the content between #{ and }.
func (e *Expander) evalDirective(inner string, ctx Context) (string, error) {
	if len(inner) == 0 {
		return "", nil
	}

	switch {
	case inner[0] == '?':
		// #{?cond,yes,no}
		return e.evalConditional(inner[1:], ctx)

	case inner[0] == '=':
		// #{=N:var} or #{=-N:var}
		return e.evalTruncation(inner[1:], ctx)

	case strings.HasPrefix(inner, "s/"):
		// #{s/pattern/replacement/:var}
		return e.evalRegexSub(inner, ctx)

	case strings.HasPrefix(inner, "||:"):
		// #{||:a,b}
		return e.evalBoolCombinator(inner[3:], ctx, false)

	case strings.HasPrefix(inner, "&&:"):
		// #{&&:a,b}
		return e.evalBoolCombinator(inner[3:], ctx, true)

	case strings.HasPrefix(inner, "T:"):
		// #{T:var} — Unix timestamp formatted as date-time
		return e.evalTimeFormat(inner[2:], ctx, "datetime")

	case strings.HasPrefix(inner, "t:"):
		// #{t:var} — Unix timestamp formatted as relative time
		return e.evalTimeFormat(inner[2:], ctx, "relative")

	default:
		// #{var} — simple variable substitution
		v, _ := ctx.Lookup(inner)
		return v, nil
	}
}

// evalConditional handles #{?cond,yes,no}.
// cond is either a variable name or a nested format expression.
// yes and no are format strings that are recursively expanded.
func (e *Expander) evalConditional(s string, ctx Context) (string, error) {
	parts := splitAtTopLevelComma(s)
	if len(parts) != 3 {
		return "", fmt.Errorf("format: conditional requires exactly 3 comma-separated parts, got %d in %q", len(parts), s)
	}
	cond, yes, no := parts[0], parts[1], parts[2]

	// Evaluate the condition: expand it if it looks like a format string,
	// otherwise treat it as a plain variable name.
	var condVal string
	var err error
	if looksLikeFormat(cond) {
		condVal, err = e.expand(cond, ctx)
		if err != nil {
			return "", err
		}
	} else {
		condVal, _ = ctx.Lookup(cond)
	}

	if isTruthy(condVal) {
		return e.expand(yes, ctx)
	}
	return e.expand(no, ctx)
}

// evalTruncation handles #{=N:var} and #{=-N:var}.
// Positive N truncates from the left (keep first N runes).
// Negative N truncates from the right (keep last N runes).
func (e *Expander) evalTruncation(s string, ctx Context) (string, error) {
	colon := strings.Index(s, ":")
	if colon < 0 {
		return "", fmt.Errorf("format: truncation directive missing ':' in %q", s)
	}
	nStr := s[:colon]
	varName := s[colon+1:]

	n, err := strconv.Atoi(nStr)
	if err != nil {
		return "", fmt.Errorf("format: invalid truncation width %q: %w", nStr, err)
	}

	val, _ := ctx.Lookup(varName)
	runes := []rune(val)

	if n >= 0 {
		if len(runes) > n {
			runes = runes[:n]
		}
	} else {
		abs := -n
		if len(runes) > abs {
			runes = runes[len(runes)-abs:]
		}
	}
	return string(runes), nil
}

// evalRegexSub handles #{s/pattern/replacement/:var} and
// #{s/pattern/replacement/flags:var} (flags are accepted but currently ignored;
// all substitutions are global).
func (e *Expander) evalRegexSub(s string, ctx Context) (string, error) {
	// s starts with "s/" — the separator is s[1].
	if len(s) < 4 {
		return "", fmt.Errorf("format: malformed regex substitution %q", s)
	}
	sep := string(s[1])
	// Split "s/<pat>/<repl>/<rest>" into at most 4 parts on the separator.
	// s[2:] strips the leading "s/".
	parts := strings.SplitN(s[2:], sep, 3)
	if len(parts) < 3 {
		return "", fmt.Errorf("format: regex substitution requires pattern, replacement, and variable in %q", s)
	}
	pattern := parts[0]
	replacement := parts[1]
	rest := parts[2] // "flags:var" or ":var"

	// Strip optional flags before the colon.
	colon := strings.Index(rest, ":")
	if colon < 0 {
		return "", fmt.Errorf("format: regex substitution missing variable after final separator in %q", s)
	}
	varName := rest[colon+1:]

	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("format: invalid regex pattern %q: %w", pattern, err)
	}

	val, _ := ctx.Lookup(varName)
	return re.ReplaceAllString(val, replacement), nil
}

// evalBoolCombinator handles #{||:a,b} and #{&&:a,b}.
// andMode=true implements &&; andMode=false implements ||.
// Each operand is a variable name or a nested format string.
// Returns "1" for true and "0" for false.
func (e *Expander) evalBoolCombinator(s string, ctx Context, andMode bool) (string, error) {
	parts := splitAtTopLevelComma(s)
	if len(parts) != 2 {
		return "", fmt.Errorf("format: boolean combinator requires exactly 2 comma-separated operands, got %d", len(parts))
	}

	vals := make([]string, 2)
	for i, part := range parts {
		var err error
		if looksLikeFormat(part) {
			vals[i], err = e.expand(part, ctx)
			if err != nil {
				return "", err
			}
		} else {
			vals[i], _ = ctx.Lookup(part)
		}
	}

	a, b := isTruthy(vals[0]), isTruthy(vals[1])
	if andMode {
		if a && b {
			return "1", nil
		}
		return "0", nil
	}
	if a || b {
		return "1", nil
	}
	return "0", nil
}

// evalTimeFormat handles #{T:var} (datetime) and #{t:var} (relative).
// var must contain a Unix timestamp as a decimal integer string.
func (e *Expander) evalTimeFormat(varName string, ctx Context, mode string) (string, error) {
	val, ok := ctx.Lookup(varName)
	if !ok || val == "" {
		return "", nil
	}

	ts, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		// Not a Unix timestamp — return the value unchanged.
		return val, nil
	}

	t := time.Unix(ts, 0)
	switch mode {
	case "datetime":
		return t.UTC().Format("2006-01-02 15:04:05"), nil
	case "relative":
		diff := time.Since(t)
		if diff < 0 {
			diff = -diff
		}
		switch {
		case diff < time.Minute:
			return fmt.Sprintf("%ds", int(diff.Seconds())), nil
		case diff < time.Hour:
			return fmt.Sprintf("%dm", int(diff.Minutes())), nil
		case diff < 24*time.Hour:
			return fmt.Sprintf("%dh", int(diff.Hours())), nil
		default:
			return fmt.Sprintf("%dd", int(diff.Hours()/24)), nil
		}
	}
	return val, nil
}

// runShellCommand splits cmd on whitespace and calls the CommandRunner.
// Returns an empty string if no runner is configured or if cmd is empty.
func (e *Expander) runShellCommand(cmd string) string {
	if e.runner == nil {
		return ""
	}
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ""
	}
	out, _ := e.runner.Run(parts[0], parts[1:])
	return strings.TrimRight(out, "\n\r")
}

// looksLikeFormat reports whether s contains a format directive (#{ or #().
func looksLikeFormat(s string) bool {
	return strings.Contains(s, "#{") || strings.Contains(s, "#(")
}

// isTruthy reports whether a value is considered truthy: non-empty and not "0".
func isTruthy(s string) bool {
	return s != "" && s != "0"
}

// splitAtTopLevelComma splits s at commas that are not enclosed within
// #{...} or #(...) constructs.
func splitAtTopLevelComma(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '{':
			depth++
		case s[i] == '}':
			if depth > 0 {
				depth--
			}
		case s[i] == '(' && i > 0 && s[i-1] == '#':
			depth++
		case s[i] == ')' && depth > 0:
			depth--
		case s[i] == ',' && depth == 0:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
