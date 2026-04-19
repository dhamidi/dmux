// Package pty opens pseudo-terminals and runs processes inside them.
//
// # Boundary
//
// The abstraction is a PTY handle with Read, Write, Resize, Start(cmd),
// Wait, and Close. Knows nothing about VT sequences, terminal state, or
// panes — it's a byte pipe to a child process with a size attribute.
//
// # Platforms
//
//   - Unix (Linux, macOS, *BSD): posix_openpt / grantpt / unlockpt /
//     ptsname, with TIOCSWINSZ for resize. The child is exec'd after
//     setsid and making the slave side its controlling tty.
//   - Windows 10 1803+: CreatePseudoConsole + two anonymous pipe pairs.
//     The child is launched with CreateProcessW + STARTUPINFOEX carrying
//     PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE_LIST. Resize is
//     ResizePseudoConsole.
//
// The build-tagged implementations live in pty_unix.go and pty_windows.go.
// The shared Go interface and any OS-agnostic helpers live in pty.go.
//
// # In isolation
//
// The package ships a small example that opens a PTY, execs $SHELL, and
// copies stdin/stdout — a standalone "shell in a shell" utility with no
// other gomux dependencies.
//
// # Non-goals
//
// No VT parsing. No scrollback. No terminal state. Those all belong
// to the libghostty-vt Terminal owned by package pane.
package pty
