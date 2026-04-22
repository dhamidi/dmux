//go:build unix

package sockpath

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// resolveDefault implements the Unix path layout:
// <tmpdir>/dmux-<uid>/<label> where tmpdir is $TMUX_TMPDIR, else
// $TMPDIR, else /tmp.
func resolveDefault(getenv func(string) string, label string) (string, error) {
	tmpdir := getenv("TMUX_TMPDIR")
	if tmpdir == "" {
		tmpdir = getenv("TMPDIR")
	}
	if tmpdir == "" {
		tmpdir = "/tmp"
	}
	uid := os.Getuid()
	dir := filepath.Join(tmpdir, fmt.Sprintf("dmux-%d", uid))
	if err := checkTmpdir(dir, uid); err != nil {
		return "", err
	}
	return filepath.Join(dir, label), nil
}

// checkTmpdir verifies an existing uid-subdir is a directory owned
// by uid and mode 0700. A non-existent dir is fine: the socket
// package creates it later with the correct perms.
func checkTmpdir(dir string, uid int) error {
	fi, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("sockpath: stat %s: %w", dir, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%w: %s: not a directory", ErrBadTmpdir, dir)
	}
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		// Every GOOS covered by //go:build unix returns *Stat_t
		// from Stat.Sys; if not, something is wrong and we refuse
		// to skip the check silently.
		return fmt.Errorf("%w: %s: stat returned %T, not *syscall.Stat_t",
			ErrBadTmpdir, dir, fi.Sys())
	}
	if int(stat.Uid) != uid {
		return fmt.Errorf("%w: %s: owned by uid %d, expected %d",
			ErrBadTmpdir, dir, stat.Uid, uid)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		return fmt.Errorf("%w: %s: mode %#o, expected 0700",
			ErrBadTmpdir, dir, perm)
	}
	return nil
}
