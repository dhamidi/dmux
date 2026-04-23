//go:build darwin

// Tests live under the darwin build tag because the pty-allocation
// dance below uses the Darwin TIOCPTY* ioctls. Linux uses TIOCSPTLCK
// / TIOCGPTN and a devpts path. The package's real implementation
// works on all unix platforms; the test helper deliberately targets
// only the dev machine per the M1 test policy.
package tty

import (
	"bytes"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// openPtyPair allocates a pty master/slave pair the test can treat
// as a fake terminal. The master stands in for "the terminal
// emulator process," the slave stands in for "the application's
// stdin+stdout." Darwin-only — this whole test file runs only on
// the dev machine per the package's test policy, so the Darwin
// TIOCPTY* path is sufficient.
//
// Deliberately duplicated from internal/pty's openpty_darwin.go:
// that copy is unexported and the tty package does not depend on
// pty. A small amount of duplication is preferable to exposing
// openpty as public API just for tests.
func openPtyPair(t *testing.T) (master, slave *os.File) {
	t.Helper()
	mfd, err := syscall.Open("/dev/ptmx", syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("open /dev/ptmx: %v", err)
	}
	if err := unix.IoctlSetInt(mfd, unix.TIOCPTYGRANT, 0); err != nil {
		syscall.Close(mfd)
		t.Fatalf("TIOCPTYGRANT: %v", err)
	}
	if err := unix.IoctlSetInt(mfd, unix.TIOCPTYUNLK, 0); err != nil {
		syscall.Close(mfd)
		t.Fatalf("TIOCPTYUNLK: %v", err)
	}
	var buf [128]byte
	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(mfd),
		uintptr(unix.TIOCPTYGNAME),
		uintptr(unsafe.Pointer(&buf[0])),
	); errno != 0 {
		syscall.Close(mfd)
		t.Fatalf("TIOCPTYGNAME: %v", errno)
	}
	n := bytes.IndexByte(buf[:], 0)
	if n < 0 {
		n = len(buf)
	}
	path := string(buf[:n])
	m := os.NewFile(uintptr(mfd), "/dev/ptmx")
	s, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		m.Close()
		t.Fatalf("open slave %s: %v", path, err)
	}
	t.Cleanup(func() {
		s.Close()
		m.Close()
	})
	return m, s
}

func setWinsize(t *testing.T, f *os.File, cols, rows int) {
	t.Helper()
	ws := &unix.Winsize{Row: uint16(rows), Col: uint16(cols)}
	if err := unix.IoctlSetWinsize(int(f.Fd()), unix.TIOCSWINSZ, ws); err != nil {
		t.Fatalf("TIOCSWINSZ: %v", err)
	}
}

func TestOpenRejectsNonTerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })
	_, err = Open(r, w)
	if err == nil {
		t.Fatal("expected error for non-tty fds")
	}
	if !errors.Is(err, ErrNotATerminal) {
		t.Fatalf("expected ErrNotATerminal, got %v", err)
	}
	var te *TTYError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TTYError, got %T", err)
	}
	if te.Op != OpOpen {
		t.Fatalf("expected Op=%s, got %s", OpOpen, te.Op)
	}
}

func TestOpenOnPtyPair(t *testing.T) {
	_, slave := openPtyPair(t)
	tty, err := Open(slave, slave)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tty.Close()
}

func TestSizeMatchesWinsize(t *testing.T) {
	_, slave := openPtyPair(t)
	setWinsize(t, slave, 120, 40)
	tty, err := Open(slave, slave)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tty.Close()

	cols, rows, err := tty.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if cols != 120 || rows != 40 {
		t.Fatalf("Size = (%d,%d), want (120,40)", cols, rows)
	}
}

func TestRawAndRestoreAreSymmetric(t *testing.T) {
	_, slave := openPtyPair(t)
	setWinsize(t, slave, 80, 24)
	before, err := unix.IoctlGetTermios(int(slave.Fd()), ioctlGetTermios)
	if err != nil {
		t.Fatalf("IoctlGetTermios pre: %v", err)
	}

	tty, err := Open(slave, slave)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tty.Close()

	if err := tty.Raw(); err != nil {
		t.Fatalf("Raw: %v", err)
	}

	afterRaw, err := unix.IoctlGetTermios(int(slave.Fd()), ioctlGetTermios)
	if err != nil {
		t.Fatalf("IoctlGetTermios raw: %v", err)
	}
	// Raw mode disables ECHO and ICANON among others; if nothing
	// changed we didn't actually apply raw.
	if afterRaw.Lflag == before.Lflag {
		t.Fatalf("Raw did not modify Lflag: before=%x after=%x", before.Lflag, afterRaw.Lflag)
	}

	if err := tty.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	afterRestore, err := unix.IoctlGetTermios(int(slave.Fd()), ioctlGetTermios)
	if err != nil {
		t.Fatalf("IoctlGetTermios restore: %v", err)
	}
	if *afterRestore != *before {
		t.Fatalf("Restore did not recover termios:\n  before=%+v\n  after =%+v", *before, *afterRestore)
	}

	// Restore again is a no-op.
	if err := tty.Restore(); err != nil {
		t.Fatalf("second Restore: %v", err)
	}
}

func TestResizeChannelFires(t *testing.T) {
	_, slave := openPtyPair(t)
	setWinsize(t, slave, 80, 24)
	tty, err := Open(slave, slave)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tty.Close()

	// Change the winsize and synthesize SIGWINCH — the OS doesn't
	// deliver SIGWINCH to us automatically when we resize a pty we
	// own, so the test has to raise it.
	setWinsize(t, slave, 132, 50)
	if err := syscall.Kill(os.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatalf("kill SIGWINCH: %v", err)
	}

	select {
	case ev := <-tty.Resize():
		if ev.Cols != 132 || ev.Rows != 50 {
			t.Fatalf("resize event = (%d,%d), want (132,50)", ev.Cols, ev.Rows)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no resize event within deadline")
	}
}

func TestCloseRestoresAndIsIdempotent(t *testing.T) {
	_, slave := openPtyPair(t)
	before, err := unix.IoctlGetTermios(int(slave.Fd()), ioctlGetTermios)
	if err != nil {
		t.Fatalf("IoctlGetTermios pre: %v", err)
	}

	tty, err := Open(slave, slave)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := tty.Raw(); err != nil {
		t.Fatalf("Raw: %v", err)
	}
	if err := tty.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	after, err := unix.IoctlGetTermios(int(slave.Fd()), ioctlGetTermios)
	if err != nil {
		t.Fatalf("IoctlGetTermios post: %v", err)
	}
	if *after != *before {
		t.Fatalf("Close did not restore termios:\n  before=%+v\n  after =%+v", *before, *after)
	}

	// Second Close returns nil and does not panic.
	if err := tty.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	// Post-close operations surface ErrClosed.
	if _, _, err := tty.Size(); !errors.Is(err, ErrClosed) {
		t.Fatalf("Size after Close: expected ErrClosed, got %v", err)
	}
	if err := tty.Raw(); !errors.Is(err, ErrClosed) {
		t.Fatalf("Raw after Close: expected ErrClosed, got %v", err)
	}
}
