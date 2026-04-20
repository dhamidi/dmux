// Package pty opens pseudo-terminals and runs processes inside them.
//
// # Interface
//
// The central type is the PTY interface:
//
//	type PTY interface {
//	    Read(p []byte) (int, error)
//	    Write(p []byte) (int, error)
//	    Resize(rows, cols int) error
//	    Close() error
//	}
//
// Callers receive a PTY through the package-level constructor:
//
//	p, err := pty.Open(cmd, args, pty.Size{Rows: 24, Cols: 80})
//
// Open starts cmd with args inside a new pseudo-terminal and returns
// the PTY interface. Read returns output from the child; Write sends
// input to it. Resize notifies the child of a window-size change.
// Close kills the child and releases OS resources.
//
// # Boundary
//
// A PTY is a byte pipe to a child process with a size attribute.
// It knows nothing about VT sequences, terminal state, or panes —
// that belongs to the libghostty-vt Terminal owned by package pane.
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
// # Testing
//
// Package pty exports FakePTY, an in-memory implementation of the PTY
// interface that requires no OS resources. Tests that depend on a PTY
// should accept a pty.PTY and use FakePTY for isolation:
//
//	f := &pty.FakePTY{}
//	f.InjectOutput([]byte("$ "))  // simulate child output
//	f.Write([]byte("ls\n"))       // simulate keyboard input
//	data := f.Input()             // verify what was written
//	sizes := f.Resizes            // verify resize history
//
// # In isolation
//
// The package ships a small example that opens a PTY, execs $SHELL, and
// copies stdin/stdout — a standalone "shell in a shell" utility with no
// other dmux dependencies.
//
// # Non-goals
//
// No VT parsing. No scrollback. No terminal state. Those all belong
// to the libghostty-vt Terminal owned by package pane.
package pty
