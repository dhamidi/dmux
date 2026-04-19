// Package shell picks a sensible default shell for spawning panes, per OS.
//
// # Boundary
//
// Default() string returns the shell executable path to use when the
// user hasn't specified one. Split out of package pane because the
// logic differs per platform and because command.Default-shell-option
// also needs to know the fallback.
//
//   - Unix: $SHELL, then /etc/passwd pw_shell, then /bin/sh.
//   - Windows: %COMSPEC%, then PowerShell, then cmd.exe.
//
// # Non-goals
//
// Does not spawn anything. Package pty spawns, package pane decides
// what argv to pass. This just answers "what should we run by default?"
package shell
