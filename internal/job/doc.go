// Package job runs short-lived helper processes asynchronously and
// delivers their output to a callback.
//
// # Boundary
//
// A Runner manages a pool of goroutines. Submit a Job and get back a
// Handle; the Runner asks its Spawner to start the process, captures
// stdout/stderr, and invokes the completion callback on a worker
// goroutine.
//
//	type Runner struct { ... }
//	func New(spawner Spawner, parallelism int) *Runner
//
//	type Spawner interface {
//	    Spawn(spec Spec) (Process, error)
//	}
//
//	type Process interface {
//	    Stdout() io.Reader
//	    Stderr() io.Reader
//	    Wait() (exitCode int, err error)
//	    Kill() error
//	}
//
//	type Spec struct {
//	    Argv []string         // explicit argv; no shell parsing here
//	    Env  []string         // explicit env; nil = empty, not inherited
//	    Dir  string           // explicit cwd; empty = parent's cwd
//	}
//
//	type Job struct {
//	    Spec    Spec
//	    Timeout time.Duration  // 0 = no timeout
//	    OnDone  func(Result)
//	}
//
//	type Result struct {
//	    Stdout   []byte
//	    Stderr   []byte
//	    ExitCode int
//	    Err      error
//	}
//
// Production wires in an os/exec-backed Spawner; tests wire in a stub
// that returns canned output without spawning anything. The Runner
// itself owns goroutines but no OS resources.
//
// # Uses
//
// Powers #(shell-cmd) in format strings, run-shell, if-shell, and
// pipe-pane's external process. NOT used for pane shells — those are
// long-running and managed by package pane directly.
//
// # I/O surfaces
//
// None directly. All process spawning is delegated to the Spawner; the
// Runner's only side effects are starting goroutines and invoking
// caller-supplied callbacks. Env and cwd are passed in by the caller.
//
// # In isolation
//
// Usable as a generic bounded parallel-command runner. A small example
// reads lines of shell commands and executes them with a configurable
// parallelism limit, printing results as they complete.
//
// # Non-goals
//
// No streaming output (callers that need live output should use
// pipe-pane instead, which hands the pane's live byte stream to a
// subprocess via package pty). No persistent job tracking across
// server restarts.
package job
