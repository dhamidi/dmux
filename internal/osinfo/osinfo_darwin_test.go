//go:build darwin

package osinfo_test

import (
	"fmt"
	"testing"

	"github.com/dhamidi/dmux/internal/osinfo"
)

// fakeProcInfo implements ProcInfo using in-memory maps, allowing tests to
// exercise Client logic without real Darwin syscalls.
type fakeProcInfo struct {
	comms map[int]string
	cwds  map[int]string
}

func (f *fakeProcInfo) Comm(pid int) (string, error) {
	if s, ok := f.comms[pid]; ok {
		return s, nil
	}
	return "", fmt.Errorf("fakeProcInfo: no comm for pid %d", pid)
}

func (f *fakeProcInfo) CWD(pid int) (string, error) {
	if s, ok := f.cwds[pid]; ok {
		return s, nil
	}
	return "", fmt.Errorf("fakeProcInfo: no cwd for pid %d", pid)
}

func newFakeProcInfo() *fakeProcInfo {
	return &fakeProcInfo{
		comms: make(map[int]string),
		cwds:  make(map[int]string),
	}
}

func TestForegroundCommand_Darwin(t *testing.T) {
	info := newFakeProcInfo()
	info.comms[42] = "vim"
	c := osinfo.New(info)
	got, err := c.ForegroundCommand(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "vim" {
		t.Errorf("ForegroundCommand: got %q, want %q", got, "vim")
	}
}

func TestForegroundCommand_Darwin_ErrorForUnknownPID(t *testing.T) {
	info := newFakeProcInfo()
	c := osinfo.New(info)
	_, err := c.ForegroundCommand(999)
	if err == nil {
		t.Error("expected error for unknown pid, got nil")
	}
}

func TestForegroundCWD_Darwin(t *testing.T) {
	info := newFakeProcInfo()
	info.cwds[42] = "/Users/alice/projects"
	c := osinfo.New(info)
	got, err := c.ForegroundCWD(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/Users/alice/projects" {
		t.Errorf("ForegroundCWD: got %q, want %q", got, "/Users/alice/projects")
	}
}

func TestForegroundCWD_Darwin_ErrorForUnknownPID(t *testing.T) {
	info := newFakeProcInfo()
	c := osinfo.New(info)
	_, err := c.ForegroundCWD(999)
	if err == nil {
		t.Error("expected error for unknown pid, got nil")
	}
}
