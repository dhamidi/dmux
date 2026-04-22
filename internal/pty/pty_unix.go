//go:build !windows

package pty

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// posixPTY is the Unix implementation of PTY backed by a real OS pseudo-terminal.
type posixPTY struct {
	master *os.File
	cmd    *exec.Cmd
	exited chan struct{} // closed when the child process exits
}

// open opens a new pseudo-terminal, starts cmd with args inside it,
// and returns a PTY handle.
func open(cmd string, args []string, size Size) (PTY, error) {
	// Open the master side of the PTY.
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("pty: open /dev/ptmx: %w", err)
	}
	masterFD := int(master.Fd())

	if err := grantpt(masterFD); err != nil {
		master.Close()
		return nil, fmt.Errorf("pty: grantpt: %w", err)
	}
	if err := unlockpt(masterFD); err != nil {
		master.Close()
		return nil, fmt.Errorf("pty: unlockpt: %w", err)
	}

	slaveName, err := ptsname(masterFD)
	if err != nil {
		master.Close()
		return nil, fmt.Errorf("pty: ptsname: %w", err)
	}

	// Open the slave side; the child process will use this as its controlling tty.
	slave, err := os.OpenFile(slaveName, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, fmt.Errorf("pty: open slave %s: %w", slaveName, err)
	}

	// Set the initial terminal size so that the child reads the correct
	// dimensions via TIOCGWINSZ on startup.  Try the slave fd first (required
	// on macOS where TIOCSWINSZ on the master returns ENOTTY), then fall back
	// to the master.
	ws := unix.Winsize{Row: uint16(size.Rows), Col: uint16(size.Cols)}
	if err := unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, &ws); err != nil {
		if err2 := unix.IoctlSetWinsize(masterFD, unix.TIOCSWINSZ, &ws); err2 != nil {
			slave.Close()
			master.Close()
			return nil, fmt.Errorf("pty: set initial winsize: %w", err2)
		}
	}

	c := exec.Command(cmd, args...)
	c.Stdin = slave
	c.Stdout = slave
	c.Stderr = slave
	c.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0, // fd 0 (stdin) becomes the controlling tty
	}
	if err := c.Start(); err != nil {
		slave.Close()
		master.Close()
		return nil, fmt.Errorf("pty: start %q: %w", cmd, err)
	}
	slave.Close() // parent does not need the slave side

	p := &posixPTY{master: master, cmd: c, exited: make(chan struct{})}
	go func() {
		_ = c.Wait()
		close(p.exited)
	}()
	return p, nil
}

// Read reads output produced by the child process.
func (p *posixPTY) Read(buf []byte) (int, error) { return p.master.Read(buf) }

// Write sends input to the child process.
func (p *posixPTY) Write(buf []byte) (int, error) { return p.master.Write(buf) }

// Resize informs the child that the terminal has been resized.
func (p *posixPTY) Resize(rows, cols int) error {
	ws := unix.Winsize{Row: uint16(rows), Col: uint16(cols)}
	return unix.IoctlSetWinsize(int(p.master.Fd()), unix.TIOCSWINSZ, &ws)
}

// Pid returns the PID of the child process started inside the PTY.
// Returns 0 if the process has not started or has exited.
func (p *posixPTY) Pid() int {
	if p.cmd.Process == nil {
		return 0
	}
	select {
	case <-p.exited:
		return 0
	default:
		return p.cmd.Process.Pid
	}
}

// Close kills the child process and releases the master PTY file descriptor.
func (p *posixPTY) Close() error {
	if p.cmd.Process != nil {
		select {
		case <-p.exited:
			// Already exited; nothing to kill.
		default:
			_ = p.cmd.Process.Kill()
			<-p.exited // wait for the goroutine to finish
		}
	}
	return p.master.Close()
}
