// Package platform isolates OS-specific process-control behavior
// that does not belong to a more focused package.
//
// # Scope
//
//   - Server spawn: re-exec the current binary as a detached server
//     child. Go cannot safely fork without exec (the runtime owns
//     threads); we achieve the tmux-style "parent keeps running as
//     client, child becomes server" split by starting a new process
//     and routing via an env var.
//   - Signals: install handlers for SIGINT, SIGTERM, SIGHUP, SIGWINCH
//     on Unix; install a console control handler on Windows and
//     translate to the same Signal values.
//   - User context: home directory and username used when spawning
//     panes and when computing socket paths.
//
// # Interface
//
//	IsServerChild() (socketPath string, ok bool)
//	SpawnServer(socketPath string) error
//	Signals(cb func(Signal))
//	HomeDir() string
//	CurrentUser() string
//
// # Server spawn protocol
//
// SpawnServer re-execs os.Executable() with DMUX_SERVER_SOCKET set
// to socketPath in the child's environment. The child is detached:
// on Unix via setsid + /dev/null for stdio; on Windows via
// DETACHED_PROCESS + CREATE_NEW_PROCESS_GROUP. SpawnServer returns
// as soon as StartProcess succeeds; the child's listener may take
// a moment longer to bind (callers poll; see internal/socket
// DialOrStart).
//
// At process startup, every dmux binary calls IsServerChild. If it
// returns (path, true), the process is the server and dispatches
// to server.Run(path). Otherwise it proceeds as a client.
//
// # Platform files
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
