package main

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/client"
	"github.com/dhamidi/dmux/internal/server"
)

// fakeSignal satisfies os.Signal for synthetic signal injection in tests.
type fakeSignal string

func (s fakeSignal) String() string { return string(s) }
func (s fakeSignal) Signal()        {}

// TestSocketPath verifies the three fallback cases for socketPath().
func TestSocketPath(t *testing.T) {
	t.Run("DMUX_SOCKET takes priority", func(t *testing.T) {
		t.Setenv("DMUX_SOCKET", "/tmp/custom.sock")
		t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
		got := socketPath()
		if got != "/tmp/custom.sock" {
			t.Errorf("socketPath() = %q, want /tmp/custom.sock", got)
		}
	})

	t.Run("XDG_RUNTIME_DIR when DMUX_SOCKET unset", func(t *testing.T) {
		t.Setenv("DMUX_SOCKET", "")
		t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
		got := socketPath()
		want := "/run/user/1000/dmux.sock"
		if got != want {
			t.Errorf("socketPath() = %q, want %q", got, want)
		}
	})

	t.Run("UserCacheDir fallback", func(t *testing.T) {
		t.Setenv("DMUX_SOCKET", "")
		t.Setenv("XDG_RUNTIME_DIR", "")
		got := socketPath()
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			t.Skipf("UserCacheDir unavailable: %v", err)
		}
		want := filepath.Join(cacheDir, "dmux", "dmux.sock")
		if got != want {
			t.Errorf("socketPath() = %q, want %q", got, want)
		}
	})
}

// TestServerStartStop starts a server on a temp Unix socket, connects a
// client using client.Run with a fixed Argv, sends a shutdown signal to
// the server, and verifies that both the server and client exit cleanly.
func TestServerStartStop(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "dmux.sock")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	sigs := make(chan os.Signal, 1)
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Run(server.Config{
			Listener: ln,
			Log:      io.Discard,
			Signals:  sigs,
		})
	}()

	// Dial the server and run a client with a fixed Argv.
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	clientCode := make(chan int, 1)
	go func() {
		clientCode <- client.Run(client.Config{
			Conn:     conn,
			In:       bytes.NewReader(nil),
			Out:      io.Discard,
			Size:     func() (int, int, error) { return 24, 80, nil },
			Argv:     []string{"list-sessions"},
			ReadOnly: true,
		})
	}()

	// Give the client time to complete the handshake.
	time.Sleep(50 * time.Millisecond)

	// Trigger graceful server shutdown via synthetic signal.
	sigs <- fakeSignal("SIGTERM")

	// The server closes connections on shutdown, causing the client to exit 0.
	select {
	case code := <-clientCode:
		if code != 0 {
			t.Errorf("client.Run() = %d, want 0", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client did not exit within timeout after server shutdown")
	}

	select {
	case err := <-serverDone:
		if err != nil {
			t.Errorf("server.Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop within timeout")
	}
}
