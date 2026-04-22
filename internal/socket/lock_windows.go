//go:build windows

package socket

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

const lockIsCrossProcess = true

// lockFile acquires an exclusive region lock on path via
// LockFileEx. It locks a single byte starting at offset 0, which
// is enough to serialize peers even though the file is zero-byte:
// the lock is on the byte range, not the file contents.
//
// This lock serializes the server cold-start handshake (see the
// package doc); it is never held during socket I/O. The lock file
// is reused across runs; we never delete it.
//
// Crash safety: LockFileEx region locks are released by the
// kernel when the handle closes, whether via UnlockFileEx or
// process termination, so a crashed starter does not orphan the
// lock.
func lockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock %s: %w", path, err)
	}
	h := windows.Handle(f.Fd())
	ol := new(windows.Overlapped)
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, ol); err != nil {
		f.Close()
		return nil, fmt.Errorf("LockFileEx %s: %w", path, err)
	}
	released := false
	return func() {
		if released {
			return
		}
		released = true
		windows.UnlockFileEx(h, 0, 1, 0, ol)
		f.Close()
	}, nil
}
