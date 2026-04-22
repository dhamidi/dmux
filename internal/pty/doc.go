// Package pty abstracts the pseudo-terminal that hosts a pane's
// child process.
//
// On Unix it uses openpty(3) plus fork/exec with the slave fd as
// stdin/stdout/stderr. On Windows it uses ConPTY:
// CreatePseudoConsole + CreateProcessW with
// PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE_HPCON.
//
// # Interface
//
//	Spawn(ctx context.Context, cfg Config) (*PTY, error)
//	(*PTY) Read(p []byte) (int, error)     // child stdout + stderr merged
//	(*PTY) Write(p []byte) (int, error)    // child stdin
//	(*PTY) Resize(cols, rows int) error
//	(*PTY) Signal(sig Signal) error        // SIGINT, SIGTERM, SIGKILL; best-effort on Windows
//	(*PTY) Wait() (ExitStatus, error)
//	(*PTY) Close() error
//
// Config carries argv, cwd, env, and initial size. Env is a full
// merged map; the caller has already combined global, session, and
// window-pane environments.
//
// # Cancellation
//
// Spawn takes a context.Context. When the context cancels, Close is
// called: it closes the pty fds, which unblocks any in-flight Read
// with an error. This is the standard pattern; Read does not
// observe contexts directly.
//
// # Windows ConPTY specifics
//
// We follow the modern API path documented by Windows Terminal and
// implemented by github.com/UserExistsError/conpty (we don't depend
// on it; we use it as a reference implementation).
//
// Exit detection: instead of polling GetExitCodeProcess, we call
// ConptyReleasePseudoConsole and read from stdout until the pipe
// returns EOF. EOF means the child has exited. This matches the
// Unix "Read returns 0/EOF when child closes its end" pattern.
//
// Async close: ConptyClosePseudoConsoleTimeout(handle, 0) provides
// non-blocking shutdown. The blocking ConptyClosePseudoConsole can
// deadlock if called on the same thread that reads from stdout
// (that thread can't drain the final frame the close emits).
//
// Resize: ResizePseudoConsole works straightforwardly when called
// outside the create-and-attach race window. Known issue
// microsoft/terminal#10400 affects callers that resize during the
// initial CreatePseudoConsole + CreateProcess window — we don't,
// because Spawn waits for both to complete before accepting Resize
// calls.
//
// Two-pipe pattern: ConPTY exposes two pipes (input and output).
// We expose Read/Write methods over them; consumer goroutines (in
// internal/pane) handle each pipe with its own goroutine to avoid
// the 1-thread-per-direction restriction Microsoft documents.
//
// # Unix specifics
//
// Standard openpty + fork pattern. SIGCHLD handling is server-wide
// (internal/platform); Wait blocks until the child has been reaped.
// Signal sends to the child process group (negative pid) so that
// shells forwarding to their children also see signals.
//
// # Scope boundary
//
//   - No VT parsing of child output; raw bytes are handed up to the
//     caller, who feeds them to a vt.Terminal.
//   - No shell lookup; the caller resolves argv (typically from
//     options.Get("default-shell") falling back to $SHELL).
//   - No process-group activity tracking. tmux uses it for activity
//     hooks; deferred to a later milestone.
//
// Platform-specific files use build tags (_unix.go, _windows.go).
// Callers never branch on GOOS.
//
// # Corresponding tmux code
//
// tmux's spawn.c plus osdep-*.c. tmux has fdforkpty as a
// platform-abstracted spawn primitive; ours is `Spawn`.
package pty
