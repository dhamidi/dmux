//go:build linux

package pty

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// grantpt ensures the slave PTY is owned by the calling process.
// On Linux with /dev/ptmx the kernel manages ownership automatically.
func grantpt(_ int) error { return nil }

// unlockpt removes the lock on the slave PTY so it can be opened.
// Linux requires an explicit TIOCSPTLCK ioctl with value 0.
// TIOCSPTLCK copies data from user space (IOC_IN), so a pointer is required.
func unlockpt(fd int) error {
	return unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0)
}

// ptsname returns the filesystem path of the slave PTY device.
// It uses TIOCGPTN to read the slave index and constructs /dev/pts/<n>.
func ptsname(fd int) (string, error) {
	n, err := unix.IoctlGetUint32(fd, unix.TIOCGPTN)
	if err != nil {
		return "", fmt.Errorf("ioctl TIOCGPTN: %w", err)
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}
