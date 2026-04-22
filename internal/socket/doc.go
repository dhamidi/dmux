// Package socket is the client<->server transport.
//
// On Unix, it uses AF_UNIX stream sockets. On Windows, it uses named
// pipes. The API is identical across platforms:
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
// via a sibling .lock file on Unix and a named mutex on Windows.
// Callers see a simple Dial-or-start flow via DialOrStart; the retry
// logic is not exposed.
//
// # Platform files
//
// socket_unix.go and socket_windows.go hold the platform-specific
// implementations. Callers never branch on GOOS.
//
// # Scope
//
// No framing (see internal/xio). No authentication or ACLs
// (tmux's server-acl.c is out of scope for milestone one;
// Unix socket permissions and Windows pipe ACLs are the only
// access control). No path resolution (see internal/sockpath).
package socket
