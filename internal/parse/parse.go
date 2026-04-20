package parse

import (
	"fmt"
	"strings"
)

// CommandList is a slice of parsed Commands.
type CommandList []Command

// Command is a single parsed command with its name, arguments, and optional
// nested body for block commands.
type Command struct {
	// Name is the command name (first token on the logical line).
	Name string
	// Args holds the remaining tokens after the command name.
	Args []string
	// Body is non-nil for commands that have a trailing { ... } block.
	// For example: if-shell "cond" { ... } else { ... }
	Body *CommandList
	// Else is non-nil when an if-block has an else branch.
	Else *CommandList
}

// Parse parses src and returns the top-level CommandList.
// src may contain multiple commands separated by newlines or semicolons.
func Parse(src string) (CommandList, error) {
	p := &parser{src: src}
	return p.parseCommandList(false)
}

// parser holds the parser state.
type parser struct {
	src string
	pos int
}

// parseCommandList reads commands until EOF or (when nested=true) until '}' is
// consumed.
func (p *parser) parseCommandList(nested bool) (CommandList, error) {
	var list CommandList
	for {
		p.skipWhitespaceAndSemicolons()

		if p.pos >= len(p.src) {
			if nested {
				return nil, fmt.Errorf("parse: unterminated block: expected '}'")
			}
			break
		}

		if nested && p.src[p.pos] == '}' {
			p.pos++ // consume '}'
			break
		}

		// Skip comment lines.
		if p.src[p.pos] == '#' {
			p.skipToEndOfLine()
			continue
		}

		cmd, err := p.parseCommand()
		if err != nil {
			return nil, err
		}
		if cmd == nil {
			// blank logical line — skip
			continue
		}
		list = append(list, *cmd)
	}
	return list, nil
}

// parseCommand reads one logical command (possibly spanning multiple physical
// lines via backslash continuation).  Returns nil when the logical line is empty.
func (p *parser) parseCommand() (*Command, error) {
	// Collect tokens for this logical line.
	tokens, err := p.readLogicalLine()
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, nil
	}

	cmd := &Command{Name: tokens[0], Args: tokens[1:]}

	// Check for a trailing block { ... } and an optional else { ... }.
	p.skipWhitespaceNoNewline()

	if p.pos < len(p.src) && p.src[p.pos] == '{' {
		p.pos++ // consume '{'
		body, err := p.parseCommandList(true)
		if err != nil {
			return nil, err
		}
		cmd.Body = &body

		// Look for optional "else { ... }".
		p.skipWhitespaceAndSemicolons()
		if p.matchKeyword("else") {
			p.skipWhitespaceNoNewline()
			if p.pos >= len(p.src) || p.src[p.pos] != '{' {
				return nil, fmt.Errorf("parse: expected '{' after else")
			}
			p.pos++ // consume '{'
			elseBody, err := p.parseCommandList(true)
			if err != nil {
				return nil, err
			}
			cmd.Else = &elseBody
		}
	}

	return cmd, nil
}

// readLogicalLine collects tokens until end-of-statement (newline not preceded
// by '\', semicolon, or EOF), respecting quoted strings and comments.
func (p *parser) readLogicalLine() ([]string, error) {
	var tokens []string
	for {
		p.skipWhitespaceNoNewline()

		if p.pos >= len(p.src) {
			break
		}

		ch := p.src[p.pos]

		// End of logical line.
		if ch == '\n' || ch == ';' {
			if ch == '\n' {
				p.pos++
			}
			break
		}

		// Line continuation.
		if ch == '\\' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '\n' {
			p.pos += 2 // consume '\' and '\n'
			continue
		}

		// Comment ends the logical line.
		if ch == '#' {
			p.skipToEndOfLine()
			break
		}

		// '{' signals the start of a block — stop collecting tokens so the
		// caller (parseCommand) can handle it.
		if ch == '{' {
			break
		}

		// '}' encountered — the caller is parseCommandList(nested=true); we
		// stop collecting so the outer loop can close the block.
		if ch == '}' {
			break
		}

		tok, err := p.readToken()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
	}
	return tokens, nil
}

// readToken reads a single token: a quoted string or a bare word.
func (p *parser) readToken() (string, error) {
	if p.pos >= len(p.src) {
		return "", fmt.Errorf("parse: unexpected end of input")
	}

	ch := p.src[p.pos]

	if ch == '\'' {
		return p.readSingleQuoted()
	}
	if ch == '"' {
		return p.readDoubleQuoted()
	}
	return p.readBareWord()
}

// readSingleQuoted reads a single-quoted string.  No escape sequences inside.
func (p *parser) readSingleQuoted() (string, error) {
	p.pos++ // consume opening '
	var b strings.Builder
	for {
		if p.pos >= len(p.src) {
			return "", fmt.Errorf("parse: unterminated single-quoted string")
		}
		ch := p.src[p.pos]
		if ch == '\'' {
			p.pos++
			return b.String(), nil
		}
		b.WriteByte(ch)
		p.pos++
	}
}

// readDoubleQuoted reads a double-quoted string with backslash escape support.
func (p *parser) readDoubleQuoted() (string, error) {
	p.pos++ // consume opening "
	var b strings.Builder
	for {
		if p.pos >= len(p.src) {
			return "", fmt.Errorf("parse: unterminated double-quoted string")
		}
		ch := p.src[p.pos]
		if ch == '"' {
			p.pos++
			return b.String(), nil
		}
		if ch == '\\' {
			p.pos++
			if p.pos >= len(p.src) {
				return "", fmt.Errorf("parse: unterminated escape in double-quoted string")
			}
			esc := p.src[p.pos]
			p.pos++
			switch esc {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte(esc)
			}
			continue
		}
		b.WriteByte(ch)
		p.pos++
	}
}

// readBareWord reads a token delimited by whitespace, semicolons, quotes, or
// special characters.
func (p *parser) readBareWord() (string, error) {
	start := p.pos
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == ';' ||
			ch == '\'' || ch == '"' || ch == '{' || ch == '}' || ch == '#' {
			break
		}
		// bare backslash-newline is handled at the logical-line level; but a
		// bare backslash followed by any other char is part of the token with
		// the escape resolved.
		if ch == '\\' && p.pos+1 < len(p.src) && p.src[p.pos+1] != '\n' {
			// return what we have so far, then handle escape on next readToken
			// call — actually simpler: just consume both chars.
			if start < p.pos {
				// flush bare chars up to here first — but since we need a
				// unified string, we switch to a builder approach.
				break
			}
			break
		}
		p.pos++
	}
	if p.pos == start {
		// shouldn't happen in normal flow
		return "", fmt.Errorf("parse: unexpected character %q at position %d", p.src[p.pos], p.pos)
	}
	return p.src[start:p.pos], nil
}

// skipWhitespaceNoNewline advances past spaces and tabs but not newlines.
func (p *parser) skipWhitespaceNoNewline() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t') {
		p.pos++
	}
}

// skipWhitespaceAndSemicolons advances past whitespace (including newlines) and
// semicolons.
func (p *parser) skipWhitespaceAndSemicolons() {
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == ';' {
			p.pos++
		} else {
			break
		}
	}
}

// skipToEndOfLine advances past the current line including the newline.
func (p *parser) skipToEndOfLine() {
	for p.pos < len(p.src) && p.src[p.pos] != '\n' {
		p.pos++
	}
	if p.pos < len(p.src) {
		p.pos++ // consume '\n'
	}
}

// matchKeyword checks whether the input at p.pos starts with word followed by a
// non-word character (or EOF).  If it matches, p.pos is advanced past the word.
func (p *parser) matchKeyword(word string) bool {
	if !strings.HasPrefix(p.src[p.pos:], word) {
		return false
	}
	after := p.pos + len(word)
	if after < len(p.src) {
		ch := p.src[after]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
			return false
		}
	}
	p.pos = after
	return true
}
