// Package osinfo answers OS-specific questions about running processes,
// primarily for pane auto-naming.
//
// # Boundary
//
// A tiny interface, satisfied by build-tagged platform implementations:
//
//	type OS interface {
//	    ForegroundCommand(pid int) (string, error)
//	    ForegroundCWD(pid int) (string, error)
//	}
//
//	func System() OS    // returns the host's implementation
//
// Given the PID of a shell running in a pane, return the command name
// of the pane's current foreground process (for `automatic-rename`) and
// its current working directory (for `new-window` with the default `-c`).
//
// Callers depend on the OS interface, not the build-tagged concrete
// types, so tests substitute a fake without #ifdefs.
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
// # I/O surfaces
//
//   - Reads the OS's process table (procfs files, sysctl, kvm, or
//     NtQueryInformationProcess depending on platform).
//   - On Linux: opens /proc/<pid>/comm and /proc/<pid>/cwd.
//
// All I/O is read-only and scoped to a pid passed in by the caller.
//
// # In isolation
//
// A standalone `dmux-osinfo PID` example prints the foreground command
// and cwd for any pid — reusable for any Go program that needs these.
//
// # Non-goals
//
// No signals, no process control, no spawning. Pure introspection.
package osinfo
