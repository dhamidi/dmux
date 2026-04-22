//go:build darwin

package pty

import (
	"bytes"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// openPty allocates a pty master/slave pair on Darwin.
//
// Darwin does not have devpts-style automatic slave setup; the
// kernel exposes three ioctls that replace grantpt/unlockpt/
// ptsname:
//
//   - TIOCPTYGRANT — equivalent of grantpt(3); adjusts slave
//     ownership to the calling user.
//   - TIOCPTYUNLK — equivalent of unlockpt(3); clears the slave
//     open-lock.
//   - TIOCPTYGNAME — equivalent of ptsname(3); copies the slave
//     device path into a 128-byte caller buffer.
//
// After the three ioctls, we open the reported path as the slave
// end. /dev/ptmx itself is the master.
//
// The master fd is put into non-blocking mode before being wrapped
// by os.NewFile. See openpty_linux.go for the rationale (runtime
// poller, Close-unblocks-Read).
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

	if err := unix.IoctlSetInt(mfd, unix.TIOCPTYGRANT, 0); err != nil {
		return nil, nil, spawnErr(OpGrantPt, ErrOpenPty, err, "")
	}
	if err := unix.IoctlSetInt(mfd, unix.TIOCPTYUNLK, 0); err != nil {
		return nil, nil, spawnErr(OpUnlockPt, ErrOpenPty, err, "")
	}

	// TIOCPTYGNAME copies a NUL-terminated path into a 128-byte
	// buffer the caller provides. There's no helper in x/sys/unix
	// for "ioctl with pointer-to-fixed-buffer", so we use the raw
	// syscall directly.
	var buf [128]byte
	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(mfd),
		uintptr(unix.TIOCPTYGNAME),
		uintptr(unsafe.Pointer(&buf[0])),
	); errno != 0 {
		return nil, nil, spawnErr(OpPtsName, ErrOpenPty, errno, "")
	}
	n := bytes.IndexByte(buf[:], 0)
	if n < 0 {
		n = len(buf)
	}
	path := string(buf[:n])

	s, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, spawnErr(OpOpenSlave, ErrOpenPty, err, "path=%s", path)
	}
	return m, s, nil
}
