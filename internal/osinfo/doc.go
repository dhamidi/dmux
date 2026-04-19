// Package osinfo answers OS-specific questions about running processes,
// primarily for pane auto-naming.
//
// # Boundary
//
// A tiny interface:
//
//	ForegroundCommand(pid int) (string, error)
//	ForegroundCWD(pid int) (string, error)
//
// Given the PID of a shell running in a pane, return the command name
// of the pane's current foreground process (for `automatic-rename`) and
// its current working directory (for `new-window` with the default `-c`).
//
// # Platforms
//
//   - Linux:   /proc/<pid>/comm, /proc/<pid>/cwd
//   - macOS:   proc_pidinfo + proc_pidpath (Darwin-specific)
//   - *BSD:    kvm_getprocs / kinfo_proc
//   - Windows: NtQueryInformationProcess for image name and cwd, plus
//              console process group enumeration to find the foreground
//              process within a ConPTY
//
// Each platform file is build-tagged.
//
// # In isolation
//
// A standalone `gomux-osinfo PID` example prints the foreground command
// and cwd for any pid — reusable for any Go program that needs these.
//
// # Non-goals
//
// No signals, no process control, no spawning. Pure introspection.
package osinfo
