//go:build darwin

package tty

import "golang.org/x/sys/unix"

// Darwin uses the BSD TIOCGETA / TIOCSETA request numbers for
// termios ioctls; Linux uses TCGETS / TCSETS. The Termios struct
// shape matches on both sides of IoctlGet/SetTermios — only the
// request number differs.
const (
	ioctlGetTermios = unix.TIOCGETA
	ioctlSetTermios = unix.TIOCSETA
)
