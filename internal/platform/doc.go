// Package platform isolates OS-specific behavior that does not belong
// to a more focused package.
//
// # Scope
//
//   - Daemonize: fork-and-detach on Unix; detached process creation on
//     Windows. Mirrors tmux's proc_fork_and_daemon from proc.c.
//   - Signals: install handlers for SIGINT, SIGTERM, SIGHUP, SIGWINCH
//     on Unix; install a console control handler on Windows and
//     translate to the same Signal values.
//   - User context: home directory and username used when spawning
//     panes and when computing socket paths.
//
// # Interface
//
//	Daemonize() (child bool, err error)
//	Signals(cb func(Signal))
//	HomeDir() string
//	CurrentUser() string
//
// Daemonize returns true in the (new) server process and false in the
// originating process; the caller dispatches accordingly.
//
// Platform-specific files use build tags (_unix.go, _windows.go).
// Callers never use GOOS branches.
//
// # Not in scope
//
//   - PTY / ConPTY handling (see internal/pty).
//   - Socket listen/dial (see internal/socket).
//   - Terminal raw mode (see internal/tty).
package platform
