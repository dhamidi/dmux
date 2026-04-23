package status

import (
	"strconv"
	"strings"
)

// View is the state the status renderer draws from. All fields are
// required; the server populates them from hardcoded values today
// (TODO(m1:status-session-name), TODO(m1:status-window-name)) and
// from the real session registry once it exists.
type View struct {
	Session    string
	WindowIdx  int
	WindowName string
	Current    bool // trailing * when true
	Cols       int  // width in cells; Render pads to exactly this
}

// Render returns the full styled cell-row bytes. No cursor motion,
// no leading/trailing newlines; the caller positions the cursor at
// the status row before writing these bytes.
//
// Shape: reverse video (ESC [ 7 m) over a space-padded
// "[<session>] <index>:<window>*" string exactly Cols cells wide,
// followed by ESC [ 0 m. Truncation: if the rendered string is
// longer than Cols, hard-truncate from the right (M1 accepts losing
// the current-window marker on narrow ttys; real tmux does similar).
//
// TODO(m1:status-runewidth): wide/double-width/emoji chars in
// session or window names are counted as 1 cell today. When the
// session registry lands and names can be arbitrary Unicode, wire
// in go-runewidth here.
func Render(v View) []byte {
	if v.Cols <= 0 {
		return nil
	}

	var content strings.Builder
	content.WriteByte('[')
	content.WriteString(v.Session)
	content.WriteString("] ")
	content.WriteString(strconv.Itoa(v.WindowIdx))
	content.WriteByte(':')
	content.WriteString(v.WindowName)
	if v.Current {
		content.WriteByte('*')
	}

	s := content.String()
	if len(s) > v.Cols {
		s = s[:v.Cols]
	} else if pad := v.Cols - len(s); pad > 0 {
		s = s + strings.Repeat(" ", pad)
	}

	var out strings.Builder
	out.Grow(len(s) + 8)
	out.WriteString("\x1b[7m")
	out.WriteString(s)
	out.WriteString("\x1b[0m")
	return []byte(out.String())
}
