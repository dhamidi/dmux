//go:build darwin

package osinfo

import (
	"bytes"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ProcInfo is the I/O abstraction for Darwin process introspection.
// Production code uses OsProcInfo; tests supply a fake implementation.
type ProcInfo interface {
	// Comm returns the short command name (up to 16 bytes) of the process.
	Comm(pid int) (string, error)
	// CWD returns the current working directory of the process.
	CWD(pid int) (string, error)
}

// OsProcInfo is the production ProcInfo backed by real Darwin syscalls.
type OsProcInfo struct{}

// Comm uses sysctl kern.proc.pid to retrieve the command name.
func (OsProcInfo) Comm(pid int) (string, error) {
	kinfo, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return "", fmt.Errorf("osinfo: SysctlKinfoProc pid %d: %w", pid, err)
	}
	comm := kinfo.Proc.P_comm
	n := bytes.IndexByte(comm[:], 0)
	if n < 0 {
		n = len(comm)
	}
	return string(comm[:n]), nil
}

// darwinVnodeInfoSize is sizeof(vnode_info) on Darwin (64-bit).
// Derived from Apple's proc_info.h:
//   sizeof(vinfo_stat)=104, vi_type=4, vi_pad=4, vi_fsid=8 → total 120.
const darwinVnodeInfoSize = 120

// darwinVnodeInfoPathSize is sizeof(vnode_info_path): vnode_info + MAXPATHLEN.
const darwinVnodeInfoPathSize = darwinVnodeInfoSize + 1024 // 1144

// darwinVnodepathInfoSize is sizeof(proc_vnodepathinfo): two vnode_info_path.
const darwinVnodepathInfoSize = 2 * darwinVnodeInfoPathSize // 2288

// procInfoCallPIDInfo is the first argument to SYS_PROC_INFO for per-pid queries.
const procInfoCallPIDInfo = 1

// procPIDVnodePathInfo is the PROC_PIDVNODEPATHINFO flavor.
const procPIDVnodePathInfo = 7

// CWD calls proc_pidinfo(PROC_PIDVNODEPATHINFO) to obtain the current
// working directory without spawning any helper processes.
func (OsProcInfo) CWD(pid int) (string, error) {
	buf := make([]byte, darwinVnodepathInfoSize)
	r1, _, errno := unix.Syscall6(
		unix.SYS_PROC_INFO,
		procInfoCallPIDInfo,
		uintptr(pid),
		procPIDVnodePathInfo,
		0, // arg
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(darwinVnodepathInfoSize),
	)
	if errno != 0 {
		return "", fmt.Errorf("osinfo: proc_pidinfo(VNODEPATHINFO) pid %d: %w", pid, errno)
	}
	if int(r1) < darwinVnodeInfoSize {
		return "", fmt.Errorf("osinfo: proc_pidinfo returned %d bytes, need at least %d", r1, darwinVnodeInfoSize)
	}
	// vip_path starts immediately after vnode_info within pvi_cdir.
	pathBytes := buf[darwinVnodeInfoSize:]
	n := bytes.IndexByte(pathBytes, 0)
	if n < 0 {
		n = 1024
	}
	return string(pathBytes[:n]), nil
}

// New returns a Client that uses info for all process introspection.
// Pass OsProcInfo{} for production and a fake in tests.
func New(info ProcInfo) *Client {
	return &Client{
		foregroundCommand: func(pid int) (string, error) {
			return info.Comm(pid)
		},
		foregroundCWD: func(pid int) (string, error) {
			return info.CWD(pid)
		},
	}
}

// Default returns a Client wired to real Darwin syscalls.
func Default() *Client { return New(OsProcInfo{}) }
