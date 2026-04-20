//go:build freebsd || openbsd || netbsd || dragonfly

package term

import "golang.org/x/sys/unix"

// ioctlReadTermios and ioctlWriteTermios are the BSD ioctl request codes
// for reading and writing terminal attributes.
const (
	ioctlReadTermios  = unix.TIOCGETA
	ioctlWriteTermios = unix.TIOCSETA
)
