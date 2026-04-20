//go:build windows

package osinfo_test

import (
	"fmt"
	"testing"

	"github.com/dhamidi/dmux/internal/osinfo"
)

// fakeNtProcess implements NtProcess using in-memory maps, allowing tests to
// exercise Client logic without real Windows API calls.
type fakeNtProcess struct {
	images map[int]string
	cwds   map[int]string
}

func (f *fakeNtProcess) QueryImageName(pid int) (string, error) {
	if s, ok := f.images[pid]; ok {
		return s, nil
	}
	return "", fmt.Errorf("fakeNtProcess: no image for pid %d", pid)
}

func (f *fakeNtProcess) QueryCurrentDirectory(pid int) (string, error) {
	if s, ok := f.cwds[pid]; ok {
		return s, nil
	}
	return "", fmt.Errorf("fakeNtProcess: no cwd for pid %d", pid)
}

func newFakeNtProcess() *fakeNtProcess {
	return &fakeNtProcess{
		images: make(map[int]string),
		cwds:   make(map[int]string),
	}
}

func TestForegroundCommand_Windows(t *testing.T) {
	nt := newFakeNtProcess()
	nt.images[42] = "vim"
	c := osinfo.New(nt)
	got, err := c.ForegroundCommand(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "vim" {
		t.Errorf("ForegroundCommand: got %q, want %q", got, "vim")
	}
}

func TestForegroundCommand_Windows_ErrorForUnknownPID(t *testing.T) {
	nt := newFakeNtProcess()
	c := osinfo.New(nt)
	_, err := c.ForegroundCommand(999)
	if err == nil {
		t.Error("expected error for unknown pid, got nil")
	}
}

func TestForegroundCWD_Windows(t *testing.T) {
	nt := newFakeNtProcess()
	nt.cwds[42] = `C:\Users\alice\projects`
	c := osinfo.New(nt)
	got, err := c.ForegroundCWD(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `C:\Users\alice\projects` {
		t.Errorf("ForegroundCWD: got %q, want %q", got, `C:\Users\alice\projects`)
	}
}

func TestForegroundCWD_Windows_ErrorForUnknownPID(t *testing.T) {
	nt := newFakeNtProcess()
	c := osinfo.New(nt)
	_, err := c.ForegroundCWD(999)
	if err == nil {
		t.Error("expected error for unknown pid, got nil")
	}
}
