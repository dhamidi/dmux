//go:build darwin

package pty

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// grantpt ensures the slave PTY device is owned by the calling user.
// On macOS the kernel grants ownership when /dev/ptmx is opened.
func grantpt(_ int) error { return nil }

// unlockpt removes the internal lock on the slave PTY.
// On macOS the slave is unlocked automatically when /dev/ptmx is opened.
func unlockpt(_ int) error { return nil }

// ptsname returns the filesystem path of the slave PTY device.
// It uses TIOCPTYGNAME to retrieve the null-terminated name from the kernel.
func ptsname(fd int) (string, error) {
	var buf [128]byte
	// TIOCPTYGNAME fills buf with the null-terminated slave device path.
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(unix.TIOCPTYGNAME), uintptr(unsafe.Pointer(&buf[0])))
	if errno != 0 {
		return "", fmt.Errorf("ioctl TIOCPTYGNAME: %w", errno)
	}
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i]), nil
		}
	}
	return string(buf[:]), nil
}
