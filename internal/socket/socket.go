package socket

import (
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

// Sentinel errors. Callers use errors.Is to dispatch on category.
var (
	// ErrAddressInUse is returned by Listen when the path names a
	// live, listening socket owned by a different process.
	ErrAddressInUse = errors.New("socket: address already in use")

	// ErrServerStartTimeout is returned by DialOrStart when the
	// startServer callback returned but the server never became
	// reachable within the dial-poll window.
	ErrServerStartTimeout = errors.New("socket: server did not come up")
)

// Tuning constants for DialOrStart's retry loop. Exported as vars
// (not consts) so tests can shorten them; package callers must not
// mutate at runtime.
var (
	dialPollInterval = 50 * time.Millisecond
	dialPollAttempts = 40 // × interval = ~2s total
)

// Listen binds an AF_UNIX stream socket at path. The file is
// unlinked on Close.
//
// A pre-existing file at path is handled as follows:
//   - If connect succeeds, a live server already owns it.
//     Returns ErrAddressInUse.
//   - If connect fails, the file is stale (prior server crashed
//     without unlinking). Listen removes it and retries bind.
//
// Listen itself does not take the cold-start lock; it assumes the
// caller already holds it, or is starting a server directly (tests,
// single-process scenarios). DialOrStart is the standard entry
// point for anything that might race with a peer.
//
// Callers must ensure the parent directory exists with the right
// permissions; see internal/sockpath.
func Listen(path string) (net.Listener, error) {
	l, err := net.Listen("unix", path)
	if err == nil {
		unlinkOnClose(l)
		return l, nil
	}
	// Bind failed. Distinguish live-socket from stale-file.
	if c, dialErr := net.Dial("unix", path); dialErr == nil {
		c.Close()
		return nil, fmt.Errorf("%w: %s", ErrAddressInUse, path)
	}
	// Stale. Remove and retry once.
	if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
		return nil, fmt.Errorf("socket: remove stale %s: %w", path, rmErr)
	}
	l, err = net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("socket: listen %s: %w", path, err)
	}
	unlinkOnClose(l)
	return l, nil
}

// Dial connects to the AF_UNIX socket at path.
func Dial(path string) (net.Conn, error) {
	c, err := net.Dial("unix", path)
	if err != nil {
		return nil, fmt.Errorf("socket: dial %s: %w", path, err)
	}
	return c, nil
}

// DialOrStart dials path if a server is already running; otherwise
// serializes the cold-start handshake via a cross-process advisory
// lock (flock on Unix, LockFileEx on Windows) so exactly one peer
// invokes startServer. See the package doc for why the lock
// matters — it is not protecting socket I/O, only the "who forks
// the server?" decision.
//
// Protocol:
//
//  1. Fast path. Dial path. On success, return — the server is
//     already running, no lock needed.
//  2. Acquire <path>.lock. Concurrent callers block here.
//  3. Re-dial. A peer may have won the lock, run startServer, and
//     released it while we were waiting. If dial now succeeds, we
//     skip step 4 entirely — no duplicate startServer call.
//  4. Call startServer. This is the one-per-cold-start fork.
//  5. Poll dial until the server accepts or the window elapses.
//     Returns ErrServerStartTimeout if it never comes up.
//  6. Release the lock (deferred).
//
// startServer's responsibility: by the time it returns nil, the
// daemon process has been spawned and its Listen call is either
// done or imminently will be. Fork-based callers typically block
// the parent on a readiness pipe that the daemon closes after its
// Listen succeeds. DialOrStart still polls briefly for robustness,
// but the expected common-case latency is one extra dial, not the
// full poll window.
//
// Polling happens even when startServer returns an error, because
// a startServer that failed partway through (log write error,
// transient ENOMEM, anything) may still have left a working
// listener — in which case the correct outcome is a successful
// connection, not an error.
func DialOrStart(path string, startServer func() error) (net.Conn, error) {
	if c, err := net.Dial("unix", path); err == nil {
		return c, nil
	}
	unlock, err := lockFile(path + ".lock")
	if err != nil {
		return nil, fmt.Errorf("socket: acquire lock: %w", err)
	}
	defer unlock()
	// A peer may have won the lock before us and already started
	// the server; check again before we do the work ourselves.
	if c, err := net.Dial("unix", path); err == nil {
		return c, nil
	}
	startErr := startServer()
	// Poll even if startServer errored: startServer is free to
	// return as soon as the daemon has been spawned, and the
	// listener may take a moment to bind. If it never binds,
	// lastErr is preserved for the final report.
	var lastErr error
	for i := 0; i < dialPollAttempts; i++ {
		if c, dialErr := net.Dial("unix", path); dialErr == nil {
			return c, nil
		} else {
			lastErr = dialErr
		}
		time.Sleep(dialPollInterval)
	}
	if startErr != nil {
		return nil, fmt.Errorf("socket: start server: %w", startErr)
	}
	return nil, fmt.Errorf("%w: %s: last dial: %v", ErrServerStartTimeout, path, lastErr)
}

func unlinkOnClose(l net.Listener) {
	if ul, ok := l.(*net.UnixListener); ok {
		ul.SetUnlinkOnClose(true)
	}
}
