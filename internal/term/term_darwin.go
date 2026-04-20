//go:build darwin

package term

import "golang.org/x/sys/unix"

// ioctlReadTermios and ioctlWriteTermios are the Darwin ioctl request codes
// for reading and writing terminal attributes.
const (
	ioctlReadTermios  = unix.TIOCGETA
	ioctlWriteTermios = unix.TIOCSETA
)
