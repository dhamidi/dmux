package sockpath

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Sentinel errors. Callers use errors.Is to dispatch on category.
var (
	// ErrInvalidLabel is returned when the -L value contains a path
	// separator. A label must name one file inside the tmpdir.
	ErrInvalidLabel = errors.New("sockpath: invalid label")

	// ErrBadTmpdir is returned when the default tmpdir subdirectory
	// exists but is not a directory, not owned by the current user,
	// or not mode 0700. Unix only.
	ErrBadTmpdir = errors.New("sockpath: bad tmpdir")

	// ErrNoLocalAppData is returned on Windows when %LOCALAPPDATA%
	// is not set and no explicit -S path was given.
	ErrNoLocalAppData = errors.New("sockpath: LOCALAPPDATA not set")
)

// Options controls socket path resolution. The zero value is valid
// and yields the default path for the current user.
type Options struct {
	// SocketPath is the -S flag value. Empty means not given; any
	// non-empty value is returned verbatim without validation.
	SocketPath string

	// Label is the -L flag value. Empty is treated as "default".
	// A label with a path separator ('/' or '\\') is rejected with
	// ErrInvalidLabel.
	Label string

	// Getenv looks up environment variables. Nil means os.Getenv.
	// Injected so tests can stage $TMPDIR, $DMUX, $LOCALAPPDATA
	// without mutating the process environment.
	Getenv func(string) string
}

// Resolve returns the socket path for the given options. It does
// not create the socket or any parent directory; it only computes
// a path and, on Unix, stats an existing tmpdir to reject it if
// permissions are wrong.
func Resolve(opts Options) (string, error) {
	if opts.SocketPath != "" {
		return opts.SocketPath, nil
	}
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	if dmux := getenv("DMUX"); dmux != "" {
		if i := strings.IndexByte(dmux, ','); i >= 0 {
			dmux = dmux[:i]
		}
		// An empty prefix ($DMUX=",17,0") falls through to the
		// default path rather than returning "": an empty path is
		// not a usable socket address.
		if dmux != "" {
			return dmux, nil
		}
	}
	label := opts.Label
	if label == "" {
		label = "default"
	}
	// Reject labels that would escape the uid-subdir or name a
	// non-regular filename. "/" and "\\" cover path traversal on
	// both platforms; "." and ".." would resolve to the subdir
	// itself or its parent; NUL is filename-illegal on Unix and
	// Windows both.
	if label == "." || label == ".." || strings.ContainsAny(label, "/\\\x00") {
		return "", fmt.Errorf("%w: %q", ErrInvalidLabel, label)
	}
	return resolveDefault(getenv, label)
}
