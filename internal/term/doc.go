// Package term drives the client's real terminal: puts it in raw mode,
// queries its size, and flushes a cell grid to it with minimal escape
// sequences.
//
// # Boundary
//
// Open(in, out *os.File) *Term — configures the tty for interactive use
// (raw mode on Unix, ENABLE_VIRTUAL_TERMINAL_PROCESSING +
// ENABLE_VIRTUAL_TERMINAL_INPUT on Windows) and remembers enough state
// to Restore() on exit.
//
// The output surface is a cell grid. Callers (package render) write cells
// via SetCell or blit a region, then Flush. Flush computes the minimum
// diff against the previously flushed frame and emits only the escape
// sequences needed to update changed cells.
//
// # Platforms
//
//   - Unix: reads terminfo for the current $TERM if available, falls
//     back to a built-in xterm-256color capability set.
//   - Windows: no terminfo. Emits xterm-compatible sequences directly,
//     relying on Windows Terminal's native VT support.
//
// Input decoding — turning escape sequences from the tty back into
// structured Key values — is NOT here. That's package keys.
//
// # In isolation
//
// A small example uses the package to play Conway's Game of Life on
// the real terminal, with no gomux state involved.
//
// # Non-goals
//
// This is NOT a terminal emulator. It only writes to a real terminal.
// Parsing escape sequences coming OUT of a child process (a pane's shell)
// is libghostty-vt's job, owned by package pane.
package term
