package log

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestServerLogPathUsesXDGStateHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-layout test")
	}
	t.Setenv("XDG_STATE_HOME", "/tmp/xyz")
	got, err := ServerLogPath("default")
	if err != nil {
		t.Fatalf("ServerLogPath: %v", err)
	}
	want := filepath.Join("/tmp/xyz", "dmux", "server-default.log")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestServerLogPathFallsBackToHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-layout test")
	}
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/tmp/abc")
	got, err := ServerLogPath("default")
	if err != nil {
		t.Fatalf("ServerLogPath: %v", err)
	}
	want := filepath.Join("/tmp/abc", ".local", "state", "dmux", "server-default.log")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestServerLogPathEmptyLabel(t *testing.T) {
	_, err := ServerLogPath("")
	if !errors.Is(err, ErrInvalidLabel) {
		t.Fatalf("got %v, want ErrInvalidLabel", err)
	}
}

func TestClientLogPathMatchesPid(t *testing.T) {
	if runtime.GOOS != "windows" {
		// Force the happy Unix branch regardless of host env. The
		// Windows branch uses LOCALAPPDATA and is covered implicitly
		// by the runtime switch.
		t.Setenv("XDG_STATE_HOME", t.TempDir())
	} else {
		t.Setenv("LOCALAPPDATA", t.TempDir())
	}
	got, err := ClientLogPath()
	if err != nil {
		t.Fatalf("ClientLogPath: %v", err)
	}
	want := fmt.Sprintf("client-%d.log", os.Getpid())
	if filepath.Base(got) != want {
		t.Fatalf("got basename %q, want %q (full path %q)", filepath.Base(got), want, got)
	}
}

func TestServerLogPathCustomLabel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-layout test")
	}
	t.Setenv("XDG_STATE_HOME", "/tmp/xyz")
	got, err := ServerLogPath("work")
	if err != nil {
		t.Fatalf("ServerLogPath: %v", err)
	}
	want := filepath.Join("/tmp/xyz", "dmux", "server-work.log")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
