//go:build linux

package term

import "golang.org/x/sys/unix"

// ioctlReadTermios and ioctlWriteTermios are the Linux ioctl request codes
// for reading and writing terminal attributes atomically.
const (
	ioctlReadTermios  = unix.TCGETS
	ioctlWriteTermios = unix.TCSETS
)
