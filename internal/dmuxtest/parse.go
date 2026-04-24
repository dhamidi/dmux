package dmuxtest

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ErrUnterminatedQuote is returned by tokenize when a scenario line
// opens a double-quoted run but never closes it. Callers can use
// errors.Is to distinguish this from other tokenize failures.
var ErrUnterminatedQuote = errors.New("dmuxtest: unterminated quote")

// tokenize splits a scenario line into argv tokens. Whitespace
// outside quotes separates tokens; "..." groups allow spaces and
// Go-style backslash escapes (\n, \t, \\, \", \r, \x1b, etc.).
// Tokens outside quotes are taken literally — no backslash
// processing, because scenarios rarely need it and a raw path is
// less surprising than one where "/tmp/x" loses its slashes.
//
// Returns nil, nil for a blank/whitespace-only line. The Play loop
// filters those before calling, but tokenize tolerates them so
// callers do not have to pre-trim.
func tokenize(line string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	inToken := false
	i := 0
	for i < len(line) {
		c := line[i]
		if c == '"' {
			// Scan to the matching close-quote. Track the raw
			// quoted chunk (including the enclosing quotes so
			// strconv.Unquote can handle escape decoding) and
			// advance past the close-quote.
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
				return nil, fmt.Errorf("dmuxtest: decode quoted token %q: %w", raw, err)
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
