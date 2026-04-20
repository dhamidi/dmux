package pty

// Size describes the terminal dimensions in characters.
type Size struct {
	Rows int
	Cols int
}

// PTY is a pseudo-terminal paired with a child process.
// The child sees a real terminal device; callers communicate
// through Read (child output) and Write (input to child).
//
// Implementations must be safe for concurrent use by a single
// reader goroutine and a single writer goroutine.
type PTY interface {
	// Read reads bytes written by the child process to its stdout/stderr.
	Read(p []byte) (int, error)
	// Write sends bytes to the child process as keyboard input.
	Write(p []byte) (int, error)
	// Resize notifies the child that the terminal window has changed size.
	Resize(rows, cols int) error
	// Close kills the child process and releases all OS resources.
	Close() error
}

// Open starts cmd with args inside a new pseudo-terminal sized to size.
// It returns a PTY handle through which callers can read the child's output
// and write input to it. The returned PTY must be closed when no longer needed.
func Open(cmd string, args []string, size Size) (PTY, error) {
	return open(cmd, args, size)
}
