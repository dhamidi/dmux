package script

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ErrUnterminatedQuote is returned by Tokenize when a script line
// opens a double-quoted run but never closes it. Callers can use
// errors.Is to distinguish this from other tokenize failures.
var ErrUnterminatedQuote = errors.New("script: unterminated quote")

// Tokenize splits a script line into argv tokens. Whitespace outside
// quotes separates tokens; "..." groups allow spaces and Go-style
// backslash escapes (\n, \t, \\, \", \r, \x1b, etc.). Tokens outside
// quotes are taken literally — no backslash processing, because
// scripts rarely need it and a raw path is less surprising than one
// where "/tmp/x" loses its slashes.
//
// Returns nil, nil for blank/whitespace-only lines and for lines
// whose first non-whitespace character is `#`. Run filters those
// before dispatching, but Tokenize is the single source of truth so
// callers do not have to pre-trim or pre-filter.
func Tokenize(line string) ([]string, error) {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" || trimmed[0] == '#' {
		return nil, nil
	}

	var tokens []string
	var cur strings.Builder
	inToken := false
	i := 0
	for i < len(line) {
		c := line[i]
		if c == '"' {
			start := i
			i++
			closed := false
			for i < len(line) {
				ch := line[i]
				if ch == '\\' && i+1 < len(line) {
					i += 2
					continue
				}
				if ch == '"' {
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("%w at offset %d: %q", ErrUnterminatedQuote, start, line)
			}
			raw := line[start:i]
			decoded, err := strconv.Unquote(raw)
			if err != nil {
				return nil, fmt.Errorf("script: decode quoted token %q: %w", raw, err)
			}
			cur.WriteString(decoded)
			inToken = true
			continue
		}
		if unicode.IsSpace(rune(c)) {
			if inToken {
				tokens = append(tokens, cur.String())
				cur.Reset()
				inToken = false
			}
			i++
			continue
		}
		cur.WriteByte(c)
		inToken = true
		i++
	}
	if inToken {
		tokens = append(tokens, cur.String())
	}
	return tokens, nil
}
