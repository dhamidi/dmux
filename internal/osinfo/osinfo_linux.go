//go:build linux

package osinfo

import (
	"fmt"
	"os"
	"strings"
)

// ProcFS is the I/O abstraction for reading from the /proc filesystem.
// Production code uses OsProcFS; tests supply a fake implementation
// to exercise parsing logic without real OS calls.
type ProcFS interface {
	// ReadFile returns the contents of the named /proc file.
	ReadFile(path string) ([]byte, error)
	// Readlink returns the destination of the named /proc symlink.
	Readlink(path string) (string, error)
}

// OsProcFS is the production ProcFS backed by the real operating system.
type OsProcFS struct{}

// ReadFile reads path using the OS filesystem.
func (OsProcFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

// Readlink resolves path as an OS symlink.
func (OsProcFS) Readlink(path string) (string, error) { return os.Readlink(path) }

// New returns a Client that uses fs for all /proc I/O.
// Pass OsProcFS{} for production and a fake in tests.
func New(fs ProcFS) *Client {
	return &Client{
		foregroundCommand: func(pid int) (string, error) {
			return linuxForegroundCommand(pid, fs)
		},
		foregroundCWD: func(pid int) (string, error) {
			return linuxForegroundCWD(pid, fs)
		},
	}
}

// Default returns a Client wired to the real /proc filesystem.
func Default() *Client { return New(OsProcFS{}) }

// linuxForegroundCommand locates the foreground process of the shell at pid
// and returns its command name.
//
// Algorithm:
//  1. Read /proc/<pid>/task/<pid>/children to find direct children.
//  2. Return the command name of the last (most recently started) child whose
//     /proc entry still exists, which is typically the foreground process.
//  3. Fall back to the shell's own command name when no children are found.
func linuxForegroundCommand(pid int, fs ProcFS) (string, error) {
	childrenPath := fmt.Sprintf("/proc/%d/task/%d/children", pid, pid)
	if data, err := fs.ReadFile(childrenPath); err == nil {
		children := strings.Fields(string(data))
		// Walk backwards: the last entry is the most recently forked child.
		for i := len(children) - 1; i >= 0; i-- {
			commPath := fmt.Sprintf("/proc/%s/comm", children[i])
			if commData, err := fs.ReadFile(commPath); err == nil {
				return strings.TrimSpace(string(commData)), nil
			}
		}
	}

	// No reachable child — the shell itself is the foreground process.
	commPath := fmt.Sprintf("/proc/%d/comm", pid)
	data, err := fs.ReadFile(commPath)
	if err != nil {
		return "", fmt.Errorf("osinfo: read comm for pid %d: %w", pid, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// linuxForegroundCWD returns the current working directory of pid
// by resolving its /proc/<pid>/cwd symlink.
func linuxForegroundCWD(pid int, fs ProcFS) (string, error) {
	cwd, err := fs.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
	if err != nil {
		return "", fmt.Errorf("osinfo: readlink cwd for pid %d: %w", pid, err)
	}
	return cwd, nil
}
