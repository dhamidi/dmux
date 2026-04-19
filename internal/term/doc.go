// Package term drives the client's real terminal: puts it in raw mode,
// queries its size, and flushes a cell grid to it with minimal escape
// sequences.
//
// # Boundary
//
//	func Open(cfg Config) (*Term, error)
//
//	type Config struct {
//	    In, Out  *os.File   // the actual tty; needs to be an *os.File
//	                        // because raw-mode setup needs a file descriptor
//	    TermName string     // value of $TERM; passed in, not read here
//	    Caps     CapSource  // looks up terminfo entries; nil = built-in
//	                        // xterm-256color fallback
//	}
//
//	type CapSource interface {
//	    Lookup(termName, capName string) (string, bool)
//	}
//
// The host process's $TERM and any terminfo database lookups are the
// caller's concern: the client process reads $TERM and constructs a
// CapSource that wraps terminfo (or whatever else), then hands both in.
// This keeps term testable without touching the filesystem.
//
// The output surface is a cell grid. Callers (package render) write cells
// via SetCell or blit a region, then Flush. Flush computes the minimum
// diff against the previously flushed frame and emits only the escape
// sequences needed to update changed cells.
//
// # Platforms
//
//   - Unix: raw mode via tcsetattr on the In file descriptor. Capability
//     strings come from the injected CapSource (typically a terminfo
//     reader) with a built-in xterm-256color fallback if no source is
//     given.
//   - Windows: no terminfo. Emits xterm-compatible sequences directly,
//     relying on Windows Terminal's native VT support. Raw mode via
//     SetConsoleMode on the In handle.
//
// # I/O surfaces
//
//   - Reads bytes from In (via callers asking for input).
//   - Writes bytes to Out.
//   - Calls tcsetattr / SetConsoleMode on the file descriptors.
//   - Calls SetConsoleOutputCP(CP_UTF8) on Windows.
//
// No environment reads, no filesystem reads (terminfo is injected), no
// network.
//
// Input decoding — turning escape sequences from the tty back into
// structured Key values — is NOT here. That's package keys.
//
// # In isolation
//
// A small example uses the package to play Conway's Game of Life on
// the real terminal, with no dmux state involved.
//
// # Non-goals
//
// This is NOT a terminal emulator. It only writes to a real terminal.
// Parsing escape sequences coming OUT of a child process (a pane's shell)
// is libghostty-vt's job, owned by package pane.
package term
