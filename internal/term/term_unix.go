//go:build !windows

package term

import (
	"os"

	"golang.org/x/sys/unix"
)

// OSSize returns a [SizeFunc] that queries the terminal dimensions of f
// using the TIOCGWINSZ ioctl. Typically f is os.Stdin or /dev/tty.
func OSSize(f *os.File) SizeFunc {
	return func() (rows, cols int, err error) {
		ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
		if err != nil {
			return 0, 0, err
		}
		return int(ws.Row), int(ws.Col), nil
	}
}

// OSRawMode returns a [RawModeFunc] that puts f into terminal raw mode
// (cfmakeraw equivalent). The returned restore function returns the
// terminal to the state it was in before OSRawMode was entered.
// Typically f is os.Stdin or /dev/tty.
func OSRawMode(f *os.File) RawModeFunc {
	return func() (func() error, error) {
		fd := int(f.Fd())
		prev, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
		if err != nil {
			return nil, err
		}

		raw := *prev
		// Disable input processing: no break signals, no CR→NL translation,
		// no parity checking, no stripping, no XON/XOFF.
		raw.Iflag &^= unix.BRKINT | unix.ICRNL | unix.INPCK | unix.ISTRIP | unix.IXON
		// Disable output post-processing.
		raw.Oflag &^= unix.OPOST
		// Set 8-bit characters, no parity.
		raw.Cflag &^= unix.CSIZE | unix.PARENB
		raw.Cflag |= unix.CS8
		// Disable canonical mode, echo, signal generation, and extended input.
		raw.Lflag &^= unix.ECHO | unix.ICANON | unix.IEXTEN | unix.ISIG
		// Read returns after at least 1 byte with no timeout.
		raw.Cc[unix.VMIN] = 1
		raw.Cc[unix.VTIME] = 0

		if err := unix.IoctlSetTermios(fd, ioctlWriteTermios, &raw); err != nil {
			return nil, err
		}
		return func() error {
			return unix.IoctlSetTermios(fd, ioctlWriteTermios, prev)
		}, nil
	}
}
