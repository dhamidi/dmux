//go:build unix

package pty

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"syscall"

	"golang.org/x/sys/unix"
)

// PTY is a handle on one running child plus its pty pair. Reads
// and writes go through the master fd; Resize, Signal, Wait, and
// Close manage the child.
//
// All methods are goroutine-safe with respect to one another.
// Concurrent calls to Read from multiple goroutines follow
// os.File's semantics (undefined interleaving of the returned
// bytes); the expected pattern is one reader goroutine per pty,
// which is what internal/pane does.
type PTY struct {
	master *os.File
	proc   *os.Process

	cancel context.CancelFunc
	closed atomic.Bool
	once   sync.Once

	waitOnce  sync.Once
	waitState *os.ProcessState
	waitErr   error
}

// Spawn forks and execs cfg.Argv under a new pty. On return, the
// child is running; reads on the returned *PTY yield the child's
// stdout/stderr, writes go to its stdin, and Wait blocks until the
// child exits.
//
// When ctx is cancelled, Close is called automatically; in-flight
// Reads unblock with an error. Spawn's own work does not observe
// ctx — it returns as soon as fork+exec completes.
func Spawn(ctx context.Context, cfg Config) (*PTY, error) {
	if len(cfg.Argv) == 0 {
		return nil, spawnErr(OpStart, ErrStartProcess, nil, "empty Argv")
	}

	master, slave, err := openPty()
	if err != nil {
		return nil, err
	}
	// On any error below, close both ends. On success, we close the
	// slave (the child has its own dup'd copy) and keep the master
	// as the pty handle.
	defer func() {
		slave.Close()
	}()
	closeMasterOnErr := true
	defer func() {
		if closeMasterOnErr {
			master.Close()
		}
	}()

	if cfg.Cols > 0 && cfg.Rows > 0 {
		ws := &unix.Winsize{
			Row: uint16(cfg.Rows),
			Col: uint16(cfg.Cols),
		}
		if err := unix.IoctlSetWinsize(int(master.Fd()), unix.TIOCSWINSZ, ws); err != nil {
			return nil, spawnErr(OpSetWinsize, ErrResize, err, "")
		}
	}

	attr := &os.ProcAttr{
		Dir:   cfg.Cwd,
		Env:   cfg.Env,
		Files: []*os.File{slave, slave, slave},
		Sys: &syscall.SysProcAttr{
			Setsid:  true, // new session — child is its own session+pgrp leader
			Setctty: true, // make Ctty the controlling terminal
			Ctty:    0,    // index into Files: the slave
		},
	}
	proc, err := os.StartProcess(cfg.Argv[0], cfg.Argv, attr)
	if err != nil {
		return nil, spawnErr(OpStart, ErrStartProcess, err, "argv0=%s", cfg.Argv[0])
	}

	childCtx, cancel := context.WithCancel(ctx)
	p := &PTY{
		master: master,
		proc:   proc,
		cancel: cancel,
	}
	go func() {
		<-childCtx.Done()
		p.Close()
	}()

	closeMasterOnErr = false
	return p, nil
}

// Read returns bytes written by the child to its stdout or stderr.
// A closed master returns an error wrapping ErrClosed.
func (p *PTY) Read(b []byte) (int, error) {
	if p.closed.Load() {
		return 0, ErrClosed
	}
	return p.master.Read(b)
}

// Write sends bytes to the child's stdin.
func (p *PTY) Write(b []byte) (int, error) {
	if p.closed.Load() {
		return 0, ErrClosed
	}
	return p.master.Write(b)
}

// Resize updates the pty's window size. The child receives SIGWINCH
// on the next kernel poll and TIOCGWINSZ reads the new values.
func (p *PTY) Resize(cols, rows int) error {
	if p.closed.Load() {
		return ErrClosed
	}
	ws := &unix.Winsize{
		Row: uint16(rows),
		Col: uint16(cols),
	}
	if err := unix.IoctlSetWinsize(int(p.master.Fd()), unix.TIOCSWINSZ, ws); err != nil {
		return spawnErr(OpResize, ErrResize, err, "cols=%d rows=%d", cols, rows)
	}
	return nil
}

// Signal delivers sig to the child's process group. Because Spawn
// ran Setsid, the child's pgid equals its pid, so sending to -pid
// reaches the child and any subprocesses that stayed in the same
// group.
func (p *PTY) Signal(sig Signal) error {
	if p.closed.Load() {
		return ErrClosed
	}
	if err := syscall.Kill(-p.proc.Pid, syscall.Signal(sig)); err != nil {
		return spawnErr(OpSignal, ErrSignal, err, "sig=%d pid=%d", sig, p.proc.Pid)
	}
	return nil
}

// Wait blocks until the child exits and returns its ExitStatus.
// Safe to call from any goroutine; subsequent calls return the
// same status.
func (p *PTY) Wait() (ExitStatus, error) {
	p.waitOnce.Do(func() {
		p.waitState, p.waitErr = p.proc.Wait()
	})
	if p.waitErr != nil {
		return ExitStatus{}, p.waitErr
	}
	return translateStatus(p.waitState), nil
}

// Close releases the pty master fd and cancels the context
// watchdog. It does not kill the child — call Signal(SIGKILL) or
// Signal(SIGHUP) first if that is desired. Closing the master on
// Unix does cause the slave to report hangup to the child, so most
// well-behaved shells will exit of their own accord shortly.
//
// Close is idempotent; repeated calls return nil.
func (p *PTY) Close() error {
	var err error
	p.once.Do(func() {
		p.closed.Store(true)
		p.cancel()
		err = p.master.Close()
	})
	return err
}

// translateStatus maps *os.ProcessState onto ExitStatus.
func translateStatus(st *os.ProcessState) ExitStatus {
	sys, ok := st.Sys().(syscall.WaitStatus)
	if !ok {
		return ExitStatus{Exited: true, Code: st.ExitCode()}
	}
	switch {
	case sys.Exited():
		return ExitStatus{Exited: true, Code: sys.ExitStatus()}
	case sys.Signaled():
		return ExitStatus{Exited: false, Signal: Signal(sys.Signal())}
	default:
		// Stopped or continued — treat as not-exited with no signal.
		return ExitStatus{}
	}
}

