// Package shell picks a sensible default shell for spawning panes, per OS.
//
// # Boundary
//
// Default(env, exists) string returns the shell executable path to use when
// the user hasn't specified one. Split out of package pane because the logic
// differs per platform and because command.Default-shell-option also needs to
// know the fallback.
//
// All OS access is explicit: callers must supply two I/O functions:
//
//   - env: func(key string) (string, bool) — looks up an environment variable.
//   - exists: func(path string) bool — reports whether a path is executable.
//
// Typical callers pass os.LookupEnv and a thin wrapper around os.Stat.
// Tests pass stubs so the package can be exercised without touching the real
// environment or filesystem.
//
// # Platform behaviour
//
//   - Unix: $SHELL → /bin/sh → "sh".
//   - Windows: %COMSPEC% → PowerShell paths → cmd.exe.
//
// # Non-goals
//
// Does not spawn anything. Package pty spawns, package pane decides
// what argv to pass. This just answers "what should we run by default?"
package shell
