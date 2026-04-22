// Package socket is the client<->server transport.
//
// Both platforms use AF_UNIX stream sockets with filesystem paths.
// Windows 10 build 17063+ and Windows Server 1809+ support AF_UNIX;
// older Windows versions are not supported. The API is identical
// across platforms:
//
//	Listen(path string) (Listener, error)
//	Dial(path string)   (net.Conn, error)
//	DialOrStart(path string, startServer func() error) (net.Conn, error)
//
// Listener returns net.Conn instances. Callers wrap the Conn with
// internal/xio for framed I/O.
//
// # Server-start race
//
// When several clients start simultaneously and no server is running,
// only one must fork the server; others must wait and then connect to
// the newly created socket. The lock-and-retry dance from tmux's
// client.c (client_get_lock + flock + retry loop) is implemented here
// via a sibling .lock file on Unix and LockFileEx on Windows.
// Callers see a simple Dial-or-start flow via DialOrStart; the retry
// logic is not exposed.
//
// # Platform files
//
// The transport is portable, but the lock primitive and any
// stat/chmod-style validation differ. Platform-specific pieces live
// in _unix.go / _windows.go files behind a shared interface;
// callers never branch on GOOS.
//
// # Scope
//
// No framing (see internal/xio). No authentication or ACLs
// (tmux's server-acl.c is out of scope for milestone one; filesystem
// permissions on the socket file and its parent directory are the
// only access control on either platform). No path resolution (see
// internal/sockpath).
package socket
