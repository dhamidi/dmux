// Command gomux is the entry point for the gomux terminal multiplexer.
//
// Depending on argv, it either:
//
//   - starts a server (`gomux start-server` or auto-started by the
//     client when the socket is unreachable), or
//   - runs as a client that connects to the server and issues a
//     command (`gomux`, `gomux new-session`, `gomux attach`, ...).
//
// The binary is a single executable containing both roles. The
// decision is made here; the work is done in internal/server and
// internal/client.
package main

import (
	"fmt"
	"os"

	// Builtins register themselves with internal/command at import
	// time. Import with _ to pull them in.
	_ "github.com/yourname/gomux/internal/command/builtin"
)

func main() {
	// TODO: argv parsing, socket path resolution, dispatch to
	// internal/server.Run or internal/client.Run.
	fmt.Fprintln(os.Stderr, "gomux: not yet implemented")
	os.Exit(1)
}
