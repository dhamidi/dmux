// Package socket is the client<->server transport.
//
// Both platforms use AF_UNIX stream sockets with filesystem paths.
// Windows 10 build 17063+ and Windows Server 1809+ support AF_UNIX;
// older Windows versions are not supported. The API is identical
// across platforms:
//
//	Listen(path string) (net.Listener, error)
//	Dial(path string)   (net.Conn, error)
//	DialOrStart(path string, startServer func() error) (net.Conn, error)
//
// Listener returns net.Conn instances. Callers wrap the Conn with
// internal/xio for framed I/O.
//
// # Server-start race and why we lock
//
// AF_UNIX I/O itself needs no coordination beyond the kernel's:
// bind(2) admits one binder per path, connect(2) against a live
// socket is atomic, and the kernel serializes accept(2). The lock
// this package takes is not protecting socket reads or writes.
//
// It exists to serialize the cold-start handshake — the moment N
// clients all start simultaneously, all see "connection refused",
// and all need to answer "who forks the server?" Without a lock,
// three things go wrong:
//
//  1. Duplicate forks. Each client races its own startServer;
//     one wins bind(2), the rest fail with EADDRINUSE mid-startup,
//     after side effects (log files, PID advertisements, goroutine
//     spawns) already happened. Expensive and noisy.
//  2. The unlink race. A crashed server leaves a socket file
//     behind; the next starter must unlink(path) before bind(path).
//     If two starters interleave unlink and bind, one can delete
//     the other's freshly bound socket.
//  3. Duplicate startup work when no fork is needed. A late
//     client arriving after another client already started the
//     server would still run startServer if it only checked
//     reachability once — then retract on EADDRINUSE. The lock +
//     re-dial pattern in DialOrStart lets the late client skip
//     startServer entirely.
//
// The lock is advisory: processes that don't cooperate with the
// protocol are not prevented from doing damage. Every dmux client
// goes through DialOrStart, so cooperation is universal in this
// codebase. tmux's client.c uses the same flock-sibling-file
// approach for the same reasons.
//
// # Platform files
//
// The transport is portable, but the lock primitive differs:
// flock(2) on Unix, LockFileEx on Windows (via x/sys/windows).
// Platform-specific pieces live in _unix.go / _windows.go files
// behind a shared internal lockFile function; callers of this
// package never branch on GOOS.
//
// # Scope
//
// No framing (see internal/xio). No authentication or ACLs
// (tmux's server-acl.c is out of scope for milestone one;
// filesystem permissions on the socket file and its parent
// directory are the only access control on either platform). No
// path resolution (see internal/sockpath).
package socket
