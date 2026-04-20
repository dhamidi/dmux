package job

import (
	"bytes"
	"context"
	"os/exec"
)

// Executor abstracts process spawning so that job logic can be tested
// without running real OS processes.
type Executor interface {
	// Run executes the named program with the given arguments, waits for it
	// to finish, and returns its combined stdout, stderr, and any error.
	Run(ctx context.Context, name string, args []string) (stdout, stderr []byte, err error)
}

// OSExecutor is the production [Executor] implementation. It spawns real
// child processes using [os/exec].
type OSExecutor struct{}

// Run implements [Executor] by creating an [exec.Cmd], running it to
// completion, and returning its captured output.
func (OSExecutor) Run(ctx context.Context, name string, args []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), err
}
