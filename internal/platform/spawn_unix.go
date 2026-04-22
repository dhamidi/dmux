//go:build unix

package platform

import (
	"fmt"
	"os"
	"syscall"
)

// SpawnServer re-execs the current binary as a detached server
// listening on socketPath. The child's env gets DMUX_SERVER_SOCKET
// so its startup dispatches to server.Run; stdio is redirected to
// /dev/null; setsid detaches it from the controlling terminal so
// the client can exit independently.
func SpawnServer(socketPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("platform: executable: %w", err)
	}
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("platform: open %s: %w", os.DevNull, err)
	}
	defer devNull.Close()

	env := append(os.Environ(), serverEnvKey+"="+socketPath)
	attr := &os.ProcAttr{
		Env:   env,
		Files: []*os.File{devNull, devNull, devNull},
		Sys:   &syscall.SysProcAttr{Setsid: true},
	}
	proc, err := os.StartProcess(exe, []string{exe}, attr)
	if err != nil {
		return fmt.Errorf("platform: start %s: %w", exe, err)
	}
	// Release: we do not wait on the child. It runs until it exits
	// on its own; the kernel reaps it via init after setsid.
	return proc.Release()
}
