// Command dmux is the entry point for the dmux terminal multiplexer.
//
// Depending on argv, it either:
//
//   - starts a server (`dmux start-server` or auto-started by the
//     client when the socket is unreachable), or
//   - runs as a client that connects to the server and issues a
//     command (`dmux`, `dmux new-session`, `dmux attach`, ...).
//
// The binary is a single executable containing both roles. The
// decision is made here; the work is done in internal/server and
// internal/client.
//
// # Socket path resolution
//
// The Unix-domain socket path is resolved in this priority order:
//
//  1. $DMUX_SOCKET environment variable (if set and non-empty)
//  2. $XDG_RUNTIME_DIR/dmux.sock (if $XDG_RUNTIME_DIR is set)
//  3. os.UserCacheDir()/dmux/dmux.sock (fallback)
//
// # Auto-start
//
// When dmux is invoked as a client but cannot reach the server socket,
// it re-executes itself as `dmux start-server` in the background and
// polls the socket until it becomes connectable (up to 2 seconds).
package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/dhamidi/dmux/internal/client"
	// Builtins register themselves with internal/command at import
	// time. Import with _ to pull them in.
	_ "github.com/dhamidi/dmux/internal/command/builtin"
	"github.com/dhamidi/dmux/internal/server"
)

// configPath returns the path to the dmux configuration file, or "" if none
// is found. It checks, in order:
//  1. $DMUX_CONFIG environment variable (if set and non-empty)
//  2. $HOME/.dmux.conf (if the file exists)
//  3. $HOME/.tmux.conf (if the file exists)
func configPath() string {
	if p := os.Getenv("DMUX_CONFIG"); p != "" {
		return p
	}
	home := os.Getenv("HOME")
	if home == "" {
		if d, err := os.UserHomeDir(); err == nil {
			home = d
		}
	}
	for _, name := range []string{".dmux.conf", ".tmux.conf"} {
		p := filepath.Join(home, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// socketPath returns the Unix-domain socket path for the dmux server,
// resolved from environment variables with a cache-dir fallback.
func socketPath() string {
	if s := os.Getenv("DMUX_SOCKET"); s != "" {
		return s
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "dmux.sock")
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	return filepath.Join(cacheDir, "dmux", "dmux.sock")
}

// runServer starts the dmux server, listening on a Unix socket at path.
// It blocks until the server exits, then returns.
func runServer(path string) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		fmt.Fprintf(os.Stderr, "dmux: mkdir: %v\n", err)
		os.Exit(1)
	}
	// Remove stale socket if no server is listening on it.
	if _, err := os.Stat(path); err == nil {
		conn, dialErr := net.Dial("unix", path)
		if dialErr != nil {
			// Nobody is listening — remove the stale socket file.
			os.Remove(path)
		} else {
			conn.Close()
			fmt.Fprintf(os.Stderr, "dmux: server already running on %s\n", path)
			os.Exit(1)
		}
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dmux: listen: %v\n", err)
		os.Exit(1)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)

	logWriter := io.Writer(os.Stderr)
	if logPath := os.Getenv("DMUX_LOG"); logPath != "" {
		if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600); err == nil {
			logWriter = io.MultiWriter(os.Stderr, f)
			defer f.Close()
		}
	}

	cfg := server.Config{
		Listener:   ln,
		Log:        logWriter,
		Signals:    sigs,
		ConfigFile: configPath(),
	}
	if err := server.Run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "dmux:", err)
		os.Exit(1)
	}
}

// autoStart launches the current executable as a background server process
// and polls the socket until it is connectable (up to 2 seconds).
func autoStart(path string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("executable: %w", err)
	}

	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open devnull: %w", err)
	}
	defer devNull.Close()

	cmd := exec.Command(exe, "start-server")
	cmd.Stdin = devNull
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start server: %w", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if conn, err := net.Dial("unix", path); err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("server did not become available within 2 seconds")
}

// dial connects to the server socket, auto-starting the server if needed.
func dial(path string) (net.Conn, error) {
	conn, err := net.Dial("unix", path)
	if err == nil {
		return conn, nil
	}
	if aerr := autoStart(path); aerr != nil {
		return nil, aerr
	}
	return net.Dial("unix", path)
}

func main() {
	path := socketPath()
	args := os.Args[1:]

	if len(args) > 0 && args[0] == "start-server" {
		runServer(path)
		return
	}

	conn, err := dial(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dmux: %v\n", err)
		os.Exit(1)
	}

	// Open /dev/tty for raw terminal I/O; fall back to os.Stdin if unavailable.
	tty, ttyErr := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	var ttyFile *os.File
	if ttyErr != nil {
		ttyFile = os.Stdin
	} else {
		ttyFile = tty
	}

	cfg := client.NewOSConfig(path, ttyFile, os.Stdout)
	cfg.Conn = conn
	cfg.Argv = args
	// Disable raw mode when /dev/tty is unavailable (not a real TTY).
	if ttyErr != nil {
		cfg.RawMode = nil
	}

	os.Exit(client.Run(cfg))
}
