// Package shell picks a sensible default shell for spawning panes, per OS.
//
// # Boundary
//
//	func Default(src Sources) string
//
//	type Sources interface {
//	    Getenv(name string) string         // wraps os.Getenv in real use
//	    LookupPasswd(uid int) (string, bool)  // unix only; nil-ok on Windows
//	    UID() int
//	}
//
// All environment access and passwd-file reads go through Sources, so
// callers (and tests) decide where the inputs come from. A real client
// passes an os-backed Sources; a test passes a stub.
//
// Default() returns the shell executable path to use when the user
// hasn't specified one. Split out of package pane because the logic
// differs per platform and because command's `default-shell` option
// also needs to know the fallback.
//
//   - Unix: Sources.Getenv("SHELL"), then LookupPasswd(UID()).pw_shell,
//     then /bin/sh.
//   - Windows: Sources.Getenv("COMSPEC"), then PowerShell, then cmd.exe.
//
// # I/O surfaces
//
// None directly. The package only inspects values returned by Sources;
// any environment / passwd / registry I/O is the Sources implementation's
// concern.
//
// # In isolation
//
// Trivially testable with a struct literal Sources. Reusable in any
// program that needs the same "what shell does this user use?" decision.
//
// # Non-goals
//
// Does not spawn anything. Package pty spawns, package pane decides
// what argv to pass. This just answers "what should we run by default?"
package shell
