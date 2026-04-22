//go:build unix

package sockpath

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckTmpdirWrongOwner(t *testing.T) {
	// We can't chown a dir to a different uid without root, but
	// checkTmpdir takes the expected uid as a parameter — passing
	// a uid the filesystem will disagree with exercises the same
	// branch a real different-uid dir would hit.
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "dmux-fake")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := checkTmpdir(dir, os.Getuid()+1)
	if !errors.Is(err, ErrBadTmpdir) {
		t.Fatalf("got %v, want ErrBadTmpdir", err)
	}
}

func TestCheckTmpdirCorrectOwner(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "dmux-real")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := checkTmpdir(dir, os.Getuid()); err != nil {
		t.Fatalf("checkTmpdir: %v", err)
	}
}
