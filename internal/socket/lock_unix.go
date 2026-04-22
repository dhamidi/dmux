//go:build unix

package socket

import (
	"fmt"
	"os"
	"syscall"
)

const lockIsCrossProcess = true

// lockFile acquires an exclusive advisory lock on path (flock(2)),
// creating the file if absent. Returns a release func that is safe
// to call multiple times.
//
// This lock serializes the server cold-start handshake (see the
// package doc); it is never held during socket I/O. The lock file
// itself is zero-byte and left on disk after release — reused
// across runs, no harm in the leftover.
//
// Crash safety: flock is kernel-released when the process dies, so
// a crashed starter does not orphan the lock. The advisory nature
// means uncooperative processes can bypass it; every dmux client
// goes through DialOrStart, so cooperation is universal.
func lockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("flock %s: %w", path, err)
	}
	released := false
	return func() {
		if released {
			return
		}
		released = true
		// Releasing via close is sufficient (flock is fd-bound);
		// the explicit LOCK_UN is defensive for fd leaks.
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
