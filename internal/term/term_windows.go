//go:build windows

package term

import (
	"os"

	"golang.org/x/sys/windows"
)

// OSSize returns a [SizeFunc] that queries the terminal dimensions of f
// using GetConsoleScreenBufferInfo. Typically f is os.Stdout or os.Stderr.
func OSSize(f *os.File) SizeFunc {
	return func() (rows, cols int, err error) {
		handle := windows.Handle(f.Fd())
		var info windows.ConsoleScreenBufferInfo
		if err := windows.GetConsoleScreenBufferInfo(handle, &info); err != nil {
			return 0, 0, err
		}
		rows = int(info.Window.Bottom-info.Window.Top) + 1
		cols = int(info.Window.Right-info.Window.Left) + 1
		return rows, cols, nil
	}
}

// OSRawMode returns a [RawModeFunc] that enables virtual-terminal input
// processing and disables line input, echo, and processed input on f.
// The returned restore function resets the console mode to its previous value.
// Typically f is os.Stdin.
func OSRawMode(f *os.File) RawModeFunc {
	return func() (func() error, error) {
		handle := windows.Handle(f.Fd())
		var prev uint32
		if err := windows.GetConsoleMode(handle, &prev); err != nil {
			return nil, err
		}
		newMode := prev
		newMode &^= windows.ENABLE_LINE_INPUT |
			windows.ENABLE_ECHO_INPUT |
			windows.ENABLE_PROCESSED_INPUT
		newMode |= windows.ENABLE_VIRTUAL_TERMINAL_INPUT
		if err := windows.SetConsoleMode(handle, newMode); err != nil {
			return nil, err
		}
		return func() error {
			return windows.SetConsoleMode(handle, prev)
		}, nil
	}
}
