// Package osinfo answers OS-specific questions about running processes,
// primarily for pane auto-naming.
//
// # Boundary
//
// The public surface is a [Client] with two methods:
//
//	(*Client).ForegroundCommand(pid int) (string, error)
//	(*Client).ForegroundCWD(pid int) (string, error)
//
// Given the PID of a shell running in a pane, ForegroundCommand returns the
// command name of the pane's current foreground process (for
// automatic-rename) and ForegroundCWD returns its current working directory
// (for new-window with the default -c flag).
//
// # Constructors and I/O Abstractions
//
// Each platform exposes a New(deps) constructor that accepts an I/O
// abstraction interface. Pass the production OS adapter for real use; pass a
// fake in tests. A Default() helper wires the production adapter for
// convenience.
//
// ## Linux
//
// New(fs ProcFS) — ProcFS has two methods:
//
//   - ReadFile(path string) ([]byte, error)  — reads a /proc file.
//   - Readlink(path string) (string, error)  — resolves a /proc symlink.
//
// The production adapter is OsProcFS, which calls os.ReadFile and os.Readlink.
// Tests supply a fakeProcFS backed by in-memory maps.
//
// ## Darwin (macOS)
//
// New(info ProcInfo) — ProcInfo has two methods:
//
//   - Comm(pid int) (string, error) — returns the short command name.
//   - CWD(pid int) (string, error)  — returns the current working directory.
//
// The production adapter is OsProcInfo, which uses sysctl(kern.proc.pid) for
// the command name and proc_pidinfo(PROC_PIDVNODEPATHINFO) for the CWD. Both
// calls are made via golang.org/x/sys/unix without spawning any subprocesses.
//
// ## BSD (FreeBSD, OpenBSD, NetBSD, DragonFly)
//
// New(info ProcInfo) — same ProcInfo interface as Darwin. The production
// adapter OsProcInfo uses sysctl(kern.proc.args) for the command name.
//
// ## Windows
//
// New(nt NtProcess) — NtProcess has two methods:
//
//   - QueryImageName(pid int) (string, error)        — returns the exe base name.
//   - QueryCurrentDirectory(pid int) (string, error) — returns the CWD.
//
// The production adapter OsNtProcess uses QueryFullProcessImageName for the
// image name and NtQueryInformationProcess + ReadProcessMemory to walk the
// RTL_USER_PROCESS_PARAMETERS structure for the current directory. No helper
// processes are spawned.
//
// # Testing in Isolation
//
// Because all OS access is channelled through the I/O abstraction, unit tests
// can supply an entirely in-memory fake without touching the real filesystem,
// sysctl, or Win32 API. See the _test.go files for each platform for concrete
// examples.
//
// # Platforms
//
//   - Linux:   /proc/<pid>/comm, /proc/<pid>/cwd, /proc/<pid>/task/<pid>/children
//   - macOS:   sysctl(kern.proc.pid) + proc_pidinfo(PROC_PIDVNODEPATHINFO)
//   - *BSD:    sysctl(kern.proc.args)
//   - Windows: QueryFullProcessImageName + NtQueryInformationProcess
//
// Each platform file is build-tagged so only the relevant code is compiled.
//
// # Non-goals
//
// No signals, no process control, no spawning. Pure introspection.
package osinfo
