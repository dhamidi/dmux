# Cross-platform notes

dmux targets Linux, macOS, and Windows 10 1803+ in Windows Terminal.
~70% of the codebase is OS-neutral Go. The rest is isolated to three
packages with build-tagged files.

## internal/pty

The biggest platform split.

Unix: `posix_openpt` / `grantpt` / `unlockpt` / `ptsname`, then
`setsid` + `TIOCSCTTY` in the child, execvp the shell. Resize via
`TIOCSWINSZ`.

Windows: `CreatePseudoConsole(size, hIn, hOut, 0, &hPC)` with two
anonymous pipe pairs. Child launch via `CreateProcessW` +
`STARTUPINFOEX` carrying `PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE_LIST`.
Resize via `ResizePseudoConsole`. No stdlib wrapper — hand-roll with
`golang.org/x/sys/windows`.

## internal/term

Unix: read terminfo for `$TERM`, fall back to a built-in
xterm-256color table. Raw mode via `tcsetattr`.

Windows: no terminfo. Emit xterm sequences directly. Raw mode via
`SetConsoleMode` with `ENABLE_VIRTUAL_TERMINAL_PROCESSING` and
`ENABLE_VIRTUAL_TERMINAL_INPUT`. Set `SetConsoleOutputCP(CP_UTF8)`.

## internal/osinfo

Linux: `/proc/<pid>/comm` and `/proc/<pid>/cwd`.

macOS: `proc_pidinfo` + `proc_pidpath`.

BSDs: `kvm_getprocs` or `sysctl(KERN_PROC_PID)`.

Windows: `NtQueryInformationProcess` for image path, `GetConsoleProcessList`
and `Toolhelp32` to walk the ConPTY's process group and find the
foreground process.

## Signals

- `SIGWINCH`: Unix only. Windows clients poll console size every
  ~100ms and send a `proto.RESIZE` when it changes. Unix clients
  use the signal as a prompt to check and send the same message.
  The protocol is uniform.
- `SIGHUP` / `SIGTERM`: Unix server shuts down gracefully. Windows
  uses `SetConsoleCtrlHandler` equivalents.
- `SIGWINCH` for the server's own terminal doesn't exist — the
  server has no terminal. Only clients have one.

## Protocol: bytes, not fds

See `docs/architecture.md`. `proto` never passes fds. File-access
features use `READ_*` / `WRITE_*` RPCs over the same socket. This is
portable by construction — AF_UNIX works on all three platforms now,
but `SCM_RIGHTS` is Unix-only.

## Clipboard

OSC 52 only. The server emits `\x1b]52;c;<base64>\x07` through the
normal output byte channel; the client's terminal puts it on the
system clipboard. Windows Terminal supports OSC 52.

## conhost (legacy Windows console)

Not supported. Document it, recommend Windows Terminal or WSL.
Supporting conhost would require a second input decoder for
`INPUT_RECORD` structs and mode translation that's not worth the
complexity.

## Cross-compilation

Pure Go packages cross-compile normally with `GOOS=windows go build`.
The cgo wrapper (`go-libghostty`) and its dependent (`internal/pane`)
need a Windows toolchain when cross-compiling. In practice: build in
CI on each target platform. `libghostty-vt` itself is built with Zig
and cross-compiles cleanly.
