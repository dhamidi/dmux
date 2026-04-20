//go:build linux

package osinfo_test

import (
	"fmt"
	"testing"

	"github.com/dhamidi/dmux/internal/osinfo"
)

// fakeProcFS implements ProcFS using in-memory maps, allowing tests to supply
// arbitrary /proc file contents and symlink destinations without touching
// the real filesystem.
type fakeProcFS struct {
	files map[string][]byte
	links map[string]string
}

func (f *fakeProcFS) ReadFile(path string) ([]byte, error) {
	if data, ok := f.files[path]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("fakeProcFS: file not found: %s", path)
}

func (f *fakeProcFS) Readlink(path string) (string, error) {
	if dest, ok := f.links[path]; ok {
		return dest, nil
	}
	return "", fmt.Errorf("fakeProcFS: symlink not found: %s", path)
}

// newFakeProcFS returns an empty fakeProcFS ready to populate.
func newFakeProcFS() *fakeProcFS {
	return &fakeProcFS{
		files: make(map[string][]byte),
		links: make(map[string]string),
	}
}

func TestForegroundCommand_ReturnsShellCommandWhenNoChildren(t *testing.T) {
	fs := newFakeProcFS()
	fs.files["/proc/100/task/100/children"] = []byte("")
	fs.files["/proc/100/comm"] = []byte("bash\n")

	c := osinfo.New(fs)
	got, err := c.ForegroundCommand(100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "bash" {
		t.Errorf("ForegroundCommand: got %q, want %q", got, "bash")
	}
}

func TestForegroundCommand_ReturnsForegroundChildCommand(t *testing.T) {
	fs := newFakeProcFS()
	fs.files["/proc/100/task/100/children"] = []byte("200 300")
	fs.files["/proc/200/comm"] = []byte("less\n")
	fs.files["/proc/300/comm"] = []byte("vim\n")
	fs.files["/proc/100/comm"] = []byte("bash\n")

	c := osinfo.New(fs)
	got, err := c.ForegroundCommand(100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The last child (300, vim) should be the foreground process.
	if got != "vim" {
		t.Errorf("ForegroundCommand: got %q, want %q", got, "vim")
	}
}

func TestForegroundCommand_SkipsDeadChildrenAndFallsBackToEarlier(t *testing.T) {
	fs := newFakeProcFS()
	// 300 is listed but has already exited (no /proc/300/comm).
	fs.files["/proc/100/task/100/children"] = []byte("200 300")
	fs.files["/proc/200/comm"] = []byte("less\n")
	// /proc/300/comm intentionally absent.
	fs.files["/proc/100/comm"] = []byte("bash\n")

	c := osinfo.New(fs)
	got, err := c.ForegroundCommand(100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Falls back to 200 (less) since 300 is gone.
	if got != "less" {
		t.Errorf("ForegroundCommand: got %q, want %q", got, "less")
	}
}

func TestForegroundCommand_FallsBackToShellWhenChildrenFileMissing(t *testing.T) {
	fs := newFakeProcFS()
	// No children file at all — process may not support task/children.
	fs.files["/proc/100/comm"] = []byte("zsh\n")

	c := osinfo.New(fs)
	got, err := c.ForegroundCommand(100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "zsh" {
		t.Errorf("ForegroundCommand: got %q, want %q", got, "zsh")
	}
}

func TestForegroundCommand_ErrorWhenCommMissing(t *testing.T) {
	fs := newFakeProcFS()
	// Neither children nor comm are present.
	c := osinfo.New(fs)
	_, err := c.ForegroundCommand(999)
	if err == nil {
		t.Error("expected error for missing pid, got nil")
	}
}

func TestForegroundCWD_ReturnsResolvedSymlink(t *testing.T) {
	fs := newFakeProcFS()
	fs.links["/proc/100/cwd"] = "/home/user/projects"

	c := osinfo.New(fs)
	got, err := c.ForegroundCWD(100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/home/user/projects" {
		t.Errorf("ForegroundCWD: got %q, want %q", got, "/home/user/projects")
	}
}

func TestForegroundCWD_ErrorWhenSymlinkMissing(t *testing.T) {
	fs := newFakeProcFS()
	c := osinfo.New(fs)
	_, err := c.ForegroundCWD(999)
	if err == nil {
		t.Error("expected error for missing cwd symlink, got nil")
	}
}

func TestForegroundCommand_TrimsTrailingNewline(t *testing.T) {
	fs := newFakeProcFS()
	fs.files["/proc/42/comm"] = []byte("fish\n")

	c := osinfo.New(fs)
	got, err := c.ForegroundCommand(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fish" {
		t.Errorf("ForegroundCommand: got %q, want %q (trailing newline not trimmed)", got, "fish")
	}
}
