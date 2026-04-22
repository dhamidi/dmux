// Package tty manages the client's real terminal device.
//
// # Scope
//
//   - Put the terminal into raw mode and restore it on exit.
//   - Read bytes from stdin into a buffer.
//   - Write bytes to stdout.
//   - Detect the initial cell size (TIOCGWINSZ on Unix,
//     GetConsoleScreenBufferInfo on Windows) and subsequent resizes.
//   - Write the mode-enable prologue at attach (CSI enables for SGR
//     mouse, bracketed paste, focus events, and KKP when the client
//     profile supports it) and the matching epilogue on detach.
//
// # Interface
//
//	Open(stdin, stdout *os.File) (*TTY, error)
//	(*TTY) Read(p []byte) (int, error)
//	(*TTY) Write(p []byte) (int, error)
//	(*TTY) Size() (cols, rows int)
//	(*TTY) OnResize(func(cols, rows int))
//	(*TTY) EnableModes(profile termcaps.Profile) error
//	(*TTY) Close() error
//
// # Scope boundary
//
// This is the client's only window onto the user's terminal. It does
// not parse bytes (bytes go straight to the server via Input frames)
// and it does not decide which capabilities to enable (that is
// internal/termcaps combined with the client's startup logic).
//
// # Corresponding tmux code
//
// tmux's tty.c, with the rendering half removed. Rendering is
// internal/termout on the server, and the bytes it produces arrive
// here only as opaque Output-frame payloads that get Write()-ed to
// stdout.
package tty
