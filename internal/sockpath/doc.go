// Package sockpath resolves the dmux server socket path.
//
// It mirrors tmux's algorithm from tmux.c, adapted for Windows:
//
//  1. If -S was given on the command line, use that path verbatim.
//  2. If $DMUX is set, take the substring before the first comma.
//  3. Otherwise, construct a path from:
//     - tmpdir: $TMUX_TMPDIR, $TMPDIR, or /tmp (Unix);
//     %LOCALAPPDATA%\dmux (Windows).
//     - uid subdir on Unix, permission-checked (0700).
//     - label: -L value, or "default" when unspecified.
//
// The result is always a filesystem path — AF_UNIX sockets work on
// both platforms (Windows 10 build 17063+, Windows Server 1809+),
// so there is no named-pipe path format to special-case. On Unix:
//
//	/tmp/dmux-1000/default
//
// On Windows:
//
//	C:\Users\<user>\AppData\Local\dmux\default
//
// # Interface
//
//	Resolve(opts Options) (string, error)
//
// Options carries the parsed -S path, -L label, and an env lookup
// function (injected for testability).
//
// # Scope
//
// This package is nearly pure. It performs stat(2) on Unix to
// validate tmpdir ownership and permissions, and nothing else. It
// does not create files, does not open sockets, and does not touch
// the network.
package sockpath
