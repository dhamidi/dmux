// Package pty opens pseudo-terminals and runs processes inside them.
//
// # Boundary
//
// The abstraction is a PTY interface:
//
//	type PTY interface {
//	    io.ReadWriteCloser
//	    Resize(cols, rows int) error
//	    Wait() (ExitStatus, error)
//	}
//
//	func Start(spec Spec) (PTY, error)
//
//	type Spec struct {
//	    Argv []string         // explicit; no $SHELL lookup here
//	    Env  []string         // explicit; no os.Environ() lookup here
//	    Dir  string           // explicit; no os.Getwd() here
//	    Cols, Rows int
//	}
//
// Knows nothing about VT sequences, terminal state, or panes — it's a
// byte pipe to a child process with a size attribute. The PTY interface
// lets pane (and tests) substitute a fake without touching real OS
// resources.
//
// # I/O surfaces
//
//   - Spawns a child process (the only I/O the package performs).
//   - Reads/writes the pty master fd (Unix) or the ConPTY pipes (Windows).
//   - Calls TIOCSWINSZ / ResizePseudoConsole on resize.
//
// All inputs (argv, env, cwd, size) are passed in by the caller. The
// package never reads the host process's environment, working directory,
// /etc/passwd, or anything else implicitly.
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
// The package ships a small example that opens a PTY and copies
// stdin/stdout — a standalone "shell in a shell" utility with no other
// dmux dependencies. The example reads $SHELL itself; the package does
// not.
//
// # Non-goals
//
// No VT parsing. No scrollback. No terminal state. Those all belong
// to the libghostty-vt Terminal owned by package pane.
package pty
