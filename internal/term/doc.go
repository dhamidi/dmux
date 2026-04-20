// Package term drives the client's real terminal: puts it in raw mode,
// queries its size, and flushes a cell grid to it with minimal escape
// sequences.
//
// # Explicit I/O Contract
//
// All I/O is injected via [Config] rather than accessed directly from
// os.Stdin/os.Stdout. Callers must supply:
//
//   - Config.In  — an [io.Reader] for raw terminal input (typically os.Stdin
//     or an open /dev/tty).
//   - Config.Out — an [io.Writer] for escape sequences (typically os.Stdout
//     or the same /dev/tty).
//   - Config.Size — a [SizeFunc] that returns the current terminal dimensions.
//     On a real terminal use [OSSize]; in tests supply a stub.
//   - Config.RawMode — a [RawModeFunc] that enters raw mode and returns a
//     restore function. On a real terminal use [OSRawMode]; may be nil in
//     tests.
//
// OS-backed implementations are provided by [OSSize] and [OSRawMode], which
// accept an *os.File (typically os.Stdin or /dev/tty).
//
// # Boundary
//
// [Open] consumes a [Config], queries the initial terminal size, optionally
// enters raw mode, and returns a *[Term].
//
// The output surface is a cell grid. Callers (package render) write cells
// via [Term.SetCell] or [Term.Clear], then call [Term.Flush]. Flush computes
// the minimum diff against the previously flushed frame and emits only the
// escape sequences needed to update changed cells.
//
// # Platforms
//
//   - Unix: [OSRawMode] uses cfmakeraw-equivalent termios manipulation via
//     golang.org/x/sys/unix. [OSSize] uses the TIOCGWINSZ ioctl.
//   - Windows: [OSRawMode] uses SetConsoleMode to enable virtual-terminal
//     input. [OSSize] uses GetConsoleScreenBufferInfo.
//
// # In isolation
//
// Because all I/O is injected, the package is fully testable without a real
// terminal: pass a *bytes.Buffer as Out, a stub SizeFunc, and nil RawMode.
//
// # Non-goals
//
// This is NOT a terminal emulator. It only writes to a real terminal.
// Parsing escape sequences coming out of a child process is libghostty-vt's
// job, owned by package pane.
package term
