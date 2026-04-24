package log

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ServerLogPath returns the log file path for a server identified by
// the given socket label. The label is the same value callers pass to
// sockpath.Resolve — an empty label is a usage bug and returns a
// wrapped ErrPathUnresolved.
func ServerLogPath(label string) (string, error) {
	if label == "" {
		return "", ErrInvalidLabel
	}
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("server-%s.log", label)), nil
}

// ClientLogPath returns the log file path for the current client
// process. One file per client pid.
func ClientLogPath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("client-%d.log", os.Getpid())), nil
}

// stateDir returns the dmux state directory for the current platform,
// without creating it. Callers pass the result to filepath.Join with
// the log file name.
func stateDir() (string, error) {
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			return "", fmt.Errorf("log: state dir: %w: LOCALAPPDATA not set", ErrPathUnresolved)
		}
		return filepath.Join(base, "dmux"), nil
	}

	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "dmux"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("log: state dir: %w: %v", ErrPathUnresolved, err)
	}
	if home == "" {
		return "", fmt.Errorf("log: state dir: %w: HOME not set", ErrPathUnresolved)
	}
	return filepath.Join(home, ".local", "state", "dmux"), nil
}
