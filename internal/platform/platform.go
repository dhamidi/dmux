package platform

import "os"

// serverEnvKey is the env-var name SpawnServer sets on the child
// and IsServerChild reads at startup. Exported as a var for tests
// that want to exercise the dispatch without re-execing.
const serverEnvKey = "DMUX_SERVER_SOCKET"

// IsServerChild reports whether the current process was started
// by SpawnServer. If so, it returns the socket path the server
// should bind. Otherwise returns ("", false) and the caller
// proceeds as a client.
//
// This is the first call in main() before flag parsing: a server
// child must not interpret the client's argv (it was re-exec'd
// with the parent's argv, which is semantically meaningless on
// this side).
func IsServerChild() (string, bool) {
	path := os.Getenv(serverEnvKey)
	return path, path != ""
}
