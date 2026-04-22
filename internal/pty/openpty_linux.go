//go:build linux

package pty

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// openPty allocates a pty master/slave pair on Linux.
//
// The sequence is standard POSIX:
//
//  1. Open /dev/ptmx with O_RDWR|O_NOCTTY|O_CLOEXEC. This is
//     posix_openpt(3) — the kernel allocates a new pty and returns
//     the master end.
//  2. unlockpt(3) via ioctl(TIOCSPTLCK, 0). Linux locks new pty
//     slaves until explicitly unlocked; this is a historical guard
//     against slave-open races during setuid programs.
//  3. Get the slave device number via ioctl(TIOCGPTN) and open
//     /dev/pts/N with O_RDWR|O_NOCTTY.
//
// grantpt(3) is a no-op on Linux with devpts (the kernel sets the
// slave's uid/gid automatically), so we skip it.
//
// The master fd is put into non-blocking mode before being wrapped
// by os.NewFile. This is what wires the *os.File to the runtime
// poller: blocking-mode files go straight to syscall.Read, which
// means a Close from another goroutine cannot unblock an in-flight
// Read. Non-blocking + poller-registered files get woken on Close.
func openPty() (master, slave *os.File, err error) {
	mfd, err := syscall.Open("/dev/ptmx", syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, spawnErr(OpOpenPtmx, ErrOpenPty, err, "")
	}
	if err := syscall.SetNonblock(mfd, true); err != nil {
		syscall.Close(mfd)
		return nil, nil, spawnErr(OpOpenPtmx, ErrOpenPty, err, "set-nonblock")
	}
	m := os.NewFile(uintptr(mfd), "/dev/ptmx")
	defer func() {
		if err != nil {
			m.Close()
		}
	}()

	// TIOCSPTLCK takes a pointer-to-int; 0 unlocks.
	if err := unix.IoctlSetPointerInt(mfd, unix.TIOCSPTLCK, 0); err != nil {
		return nil, nil, spawnErr(OpUnlockPt, ErrOpenPty, err, "")
	}

	n, err := unix.IoctlGetInt(mfd, unix.TIOCGPTN)
	if err != nil {
		return nil, nil, spawnErr(OpPtsName, ErrOpenPty, err, "")
	}

	path := fmt.Sprintf("/dev/pts/%d", n)
	s, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, spawnErr(OpOpenSlave, ErrOpenPty, err, "path=%s", path)
	}
	return m, s, nil
}
