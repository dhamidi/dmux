package status

import (
	"strconv"
	"strings"
)

// View is the state the status renderer draws from. Session is the
// name shown in the left label; Windows is the per-window list
// (rendered tmux-style as "idx:name" with a trailing "*" on the
// current window); Cols is the tty width the caller needs the
// rendered bytes padded to.
type View struct {
	Session string
	Windows []WindowSlot
	Cols    int
}

// WindowSlot is one entry in the status bar's window list. Idx is the
// window's stable index (not its slice position — windows retain
// their original index when earlier windows are closed, matching
// tmux). Name is the display name. Current marks the session's
// current window with a trailing "*".
type WindowSlot struct {
	Idx     int
	Name    string
	Current bool
}

// Render returns the full styled cell-row bytes. No cursor motion,
// no leading/trailing newlines; the caller positions the cursor at
// the status row before writing these bytes.
//
// Shape: reverse video (ESC [ 7 m) over a space-padded
// "[<session>] <idx>:<name>[*] <idx>:<name>[*]..." string exactly
// Cols cells wide, followed by ESC [ 0 m. Truncation: if the
// rendered string is longer than Cols, hard-truncate from the right
// (M1 accepts losing the current-window marker on narrow ttys; real
// tmux does similar).
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
	content.WriteByte(']')
	for _, w := range v.Windows {
		content.WriteByte(' ')
		content.WriteString(strconv.Itoa(w.Idx))
		content.WriteByte(':')
		content.WriteString(w.Name)
		if w.Current {
			content.WriteByte('*')
		}
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
