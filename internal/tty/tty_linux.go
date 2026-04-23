//go:build linux

package tty

import "golang.org/x/sys/unix"

// Linux uses TCGETS / TCSETS for termios ioctls; Darwin uses the
// BSD TIOCGETA / TIOCSETA numbers. The Termios struct shape
// matches on both sides of IoctlGet/SetTermios — only the request
// number differs.
const (
	ioctlGetTermios = unix.TCGETS
	ioctlSetTermios = unix.TCSETS
)
