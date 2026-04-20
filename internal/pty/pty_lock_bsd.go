//go:build freebsd || dragonfly || netbsd || openbsd

package pty

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// grantpt ensures the slave PTY device is owned by the calling user.
// On BSD systems the kernel grants ownership when /dev/ptmx is opened.
func grantpt(_ int) error { return nil }

// unlockpt removes the internal lock on the slave PTY.
// On BSD systems the slave is unlocked automatically when /dev/ptmx is opened.
func unlockpt(_ int) error { return nil }

// ptsname returns the filesystem path of the slave PTY device.
// FreeBSD and DragonFly use TIOCGPTN to obtain the slave device index.
// NetBSD and OpenBSD use legacy pty naming; TIOCGPTN may not be available.
func ptsname(fd int) (string, error) {
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		return "", fmt.Errorf("ioctl TIOCGPTN: %w", err)
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}
