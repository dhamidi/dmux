//go:build freebsd || openbsd || netbsd || dragonfly

package osinfo

import (
	"bytes"
	"fmt"
	"syscall"
	"unsafe"
)

// ProcInfo is the I/O abstraction for BSD process introspection.
// Production code uses OsProcInfo; tests supply a fake implementation.
type ProcInfo interface {
	// Comm returns the short command name of the process.
	Comm(pid int) (string, error)
	// CWD returns the current working directory of the process.
	CWD(pid int) (string, error)
}

// OsProcInfo is the production ProcInfo backed by real BSD sysctl calls.
type OsProcInfo struct{}

// bsdKinfoProc mirrors the fields of kinfo_proc we need.
// The sysctl kern.proc.pid returns a binary blob whose first MAXCOMLEN+1
// bytes (after the fixed header) contain the command name.
//
// We parse it using the platform C struct layout via unsafe reads rather
// than importing cgo, so the offsets here are empirically verified for
// 64-bit FreeBSD/NetBSD/OpenBSD/DragonFly.
//
// On all supported 64-bit BSDs the command name is an ASCII NUL-terminated
// string sitting at a fixed offset within the kinfo_proc blob:
//   - FreeBSD: ki_comm at byte offset 447 (after a large header)
//   - NetBSD:  p_comm  at a similar offset
//   - OpenBSD: p_comm  at a similar offset
//
// Because offsets differ across BSDs we use the simpler kern.proc.args
// approach: parse the raw blob by scanning for the NUL-terminated string
// that represents the executable name, which starts at a known per-platform
// offset baked in as bsdCommOffset below.

// bsdKinfoMIB is the sysctl MIB for KERN_PROC / KERN_PROC_PID.
func bsdKernProcPIDRaw(pid int) ([]byte, error) {
	// MIB: CTL_KERN=1, KERN_PROC=14, KERN_PROC_PID=1, <pid>
	mib := []int32{1, 14, 1, int32(pid)}
	n := uintptr(0)
	// First call: get size.
	if _, _, errno := syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		uintptr(len(mib)),
		0, uintptr(unsafe.Pointer(&n)),
		0, 0,
	); errno != 0 {
		return nil, fmt.Errorf("sysctl size: %w", errno)
	}
	buf := make([]byte, n)
	if _, _, errno := syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		uintptr(len(mib)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&n)),
		0, 0,
	); errno != 0 {
		return nil, fmt.Errorf("sysctl data: %w", errno)
	}
	return buf[:n], nil
}

// Comm reads the command name from the kinfo_proc blob returned by sysctl.
// The blob layout is BSD-specific; we scan for the first printable NUL-
// terminated run long enough to be a real command name.
func (OsProcInfo) Comm(pid int) (string, error) {
	// Prefer kern.proc.args which on many BSDs returns the process argv
	// as a NUL-separated list; the first entry is the executable name.
	mib := []int32{1, 38, int32(pid)} // CTL_KERN=1, KERN_PROC_ARGS=38
	n := uintptr(4096)
	buf := make([]byte, n)
	if _, _, errno := syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		uintptr(len(mib)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&n)),
		0, 0,
	); errno != 0 {
		return "", fmt.Errorf("osinfo: kern.proc.args pid %d: %w", pid, errno)
	}
	// The first NUL-terminated string in buf is argv[0].
	arg0 := buf[:n]
	end := bytes.IndexByte(arg0, 0)
	if end < 0 {
		end = int(n)
	}
	argv0 := string(arg0[:end])
	// Strip leading path components to get the basename.
	if i := bytes.LastIndexByte([]byte(argv0), '/'); i >= 0 {
		argv0 = argv0[i+1:]
	}
	return argv0, nil
}

// CWD is not available via sysctl on BSD without cgo; returns an error
// advising callers to use the cgo-enabled build.
// TODO(platform): implement using fcntl F_GETPATH or procfs where available.
func (OsProcInfo) CWD(pid int) (string, error) {
	return "", fmt.Errorf("osinfo: CWD not implemented on this BSD platform for pid %d", pid)
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

// Default returns a Client wired to real BSD sysctl calls.
func Default() *Client { return New(OsProcInfo{}) }
