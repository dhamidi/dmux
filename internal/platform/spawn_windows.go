//go:build windows

package platform

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

// SpawnServer re-execs the current binary as a detached server
// listening on socketPath. The child's env gets DMUX_SERVER_SOCKET
// so its startup dispatches to server.Run; DETACHED_PROCESS plus
// CREATE_NEW_PROCESS_GROUP keep the child alive after the client
// exits and give it its own console-control-event scope.
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
		Sys: &syscall.SysProcAttr{
			CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
			HideWindow:    true,
		},
	}
	proc, err := os.StartProcess(exe, []string{exe}, attr)
	if err != nil {
		return fmt.Errorf("platform: start %s: %w", exe, err)
	}
	return proc.Release()
}
