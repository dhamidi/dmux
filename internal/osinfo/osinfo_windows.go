//go:build windows

package osinfo

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NtProcess is the I/O abstraction for Windows process introspection.
// Production code uses OsNtProcess; tests supply a fake implementation.
type NtProcess interface {
	// QueryImageName returns the executable base name of the process.
	QueryImageName(pid int) (string, error)
	// QueryCurrentDirectory returns the current working directory of the process.
	QueryCurrentDirectory(pid int) (string, error)
}

// OsNtProcess is the production NtProcess backed by real Windows APIs.
type OsNtProcess struct{}

var (
	modNtdll                      = windows.NewLazySystemDLL("ntdll.dll")
	procNtQueryInformationProcess = modNtdll.NewProc("NtQueryInformationProcess")
)

// processBasicInformation mirrors PROCESS_BASIC_INFORMATION from winternl.h.
type processBasicInformation struct {
	ExitStatus                   uintptr
	PebBaseAddress               uintptr
	AffinityMask                 uintptr
	BasePriority                 uintptr
	UniqueProcessID              uintptr
	InheritedFromUniqueProcessID uintptr
}

// QueryImageName opens the process and reads its image file name using
// QueryFullProcessImageName, then strips the path to return the base name.
func (OsNtProcess) QueryImageName(pid int) (string, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", fmt.Errorf("osinfo: OpenProcess pid %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)

	buf := make([]uint16, windows.MAX_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return "", fmt.Errorf("osinfo: QueryFullProcessImageName pid %d: %w", pid, err)
	}
	full := windows.UTF16ToString(buf[:n])
	// Extract the base name (after last backslash).
	base := full
	for i := len(full) - 1; i >= 0; i-- {
		if full[i] == '\\' || full[i] == '/' {
			base = full[i+1:]
			break
		}
	}
	// Strip .exe suffix if present.
	if len(base) > 4 && base[len(base)-4:] == ".exe" {
		base = base[:len(base)-4]
	}
	return base, nil
}

// rtlUserProcessParameters offset within PEB (64-bit Windows).
const pebRtlUserProcessParamsOffset = 0x20

// QueryCurrentDirectory reads the CurrentDirectory field from the process's
// RTL_USER_PROCESS_PARAMETERS structure via NtQueryInformationProcess.
func (OsNtProcess) QueryCurrentDirectory(pid int) (string, error) {
	h, err := windows.OpenProcess(
		windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ,
		false,
		uint32(pid),
	)
	if err != nil {
		return "", fmt.Errorf("osinfo: OpenProcess pid %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)

	// Get PEB address via NtQueryInformationProcess(ProcessBasicInformation=0).
	var pbi processBasicInformation
	var returnLen uint32
	r, _, _ := procNtQueryInformationProcess.Call(
		uintptr(h),
		0, // ProcessBasicInformation
		uintptr(unsafe.Pointer(&pbi)),
		uintptr(unsafe.Sizeof(pbi)),
		uintptr(unsafe.Pointer(&returnLen)),
	)
	if r != 0 {
		return "", fmt.Errorf("osinfo: NtQueryInformationProcess pid %d: status 0x%x", pid, r)
	}

	// Read the RTL_USER_PROCESS_PARAMETERS pointer from the PEB.
	var paramsAddr uintptr
	if err := windows.ReadProcessMemory(h,
		pbi.PebBaseAddress+pebRtlUserProcessParamsOffset,
		(*byte)(unsafe.Pointer(&paramsAddr)),
		unsafe.Sizeof(paramsAddr),
		nil,
	); err != nil {
		return "", fmt.Errorf("osinfo: ReadProcessMemory PEB pid %d: %w", pid, err)
	}

	// RTL_USER_PROCESS_PARAMETERS.CurrentDirectory is a CURDIR structure at
	// offset 0x38 (64-bit). CURDIR = { UNICODE_STRING DosPath; HANDLE Handle }.
	// UNICODE_STRING = { USHORT Length; USHORT MaximumLength; PWSTR Buffer }.
	const curDirOffset = 0x38
	var length uint16
	var bufPtr uintptr

	if err := windows.ReadProcessMemory(h,
		paramsAddr+curDirOffset,
		(*byte)(unsafe.Pointer(&length)),
		2,
		nil,
	); err != nil {
		return "", fmt.Errorf("osinfo: ReadProcessMemory CurrentDirectory length pid %d: %w", pid, err)
	}
	if err := windows.ReadProcessMemory(h,
		paramsAddr+curDirOffset+8, // Buffer field at offset 8 within UNICODE_STRING
		(*byte)(unsafe.Pointer(&bufPtr)),
		unsafe.Sizeof(bufPtr),
		nil,
	); err != nil {
		return "", fmt.Errorf("osinfo: ReadProcessMemory CurrentDirectory buffer pid %d: %w", pid, err)
	}

	if length == 0 || bufPtr == 0 {
		return "", fmt.Errorf("osinfo: empty CurrentDirectory for pid %d", pid)
	}

	dirBuf := make([]uint16, length/2)
	if err := windows.ReadProcessMemory(h,
		bufPtr,
		(*byte)(unsafe.Pointer(&dirBuf[0])),
		uintptr(length),
		nil,
	); err != nil {
		return "", fmt.Errorf("osinfo: ReadProcessMemory CurrentDirectory data pid %d: %w", pid, err)
	}
	path := windows.UTF16ToString(dirBuf)
	// Strip trailing backslash except for drive roots (e.g. "C:\").
	if len(path) > 3 && path[len(path)-1] == '\\' {
		path = path[:len(path)-1]
	}
	return path, nil
}

// New returns a Client that uses nt for all process introspection.
// Pass OsNtProcess{} for production and a fake in tests.
func New(nt NtProcess) *Client {
	return &Client{
		foregroundCommand: func(pid int) (string, error) {
			return nt.QueryImageName(pid)
		},
		foregroundCWD: func(pid int) (string, error) {
			return nt.QueryCurrentDirectory(pid)
		},
	}
}

// Default returns a Client wired to real Windows APIs.
func Default() *Client { return New(OsNtProcess{}) }
