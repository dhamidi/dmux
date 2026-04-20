// Package job runs short-lived helper processes asynchronously and
// delivers their output to a callback.
//
// # Boundary
//
// A Runner manages a pool of goroutines. Submit a Job and get back a
// Handle; the Runner spawns the process via the injected [Executor],
// captures stdout/stderr, and invokes the completion callback on a
// worker goroutine.
//
//	type Job struct {
//	    Command  string         // sh -c "..." style
//	    Timeout  time.Duration  // 0 = no timeout
//	    OnDone   func(Result)
//	}
//
//	type Result struct {
//	    Stdout   []byte
//	    Stderr   []byte
//	    ExitCode int
//	    Err      error
//	}
//
// # Executor interface
//
// Process spawning is abstracted behind [Executor]:
//
//	type Executor interface {
//	    Run(ctx context.Context, name string, args []string) (stdout, stderr []byte, err error)
//	}
//
// The production implementation is [OSExecutor], which calls
// [os/exec.Command] under the hood. Tests can substitute a fake
// implementation that returns canned output without spawning real
// processes.
//
// # Constructor
//
// Create a Runner with [NewRunner]:
//
//	r := job.NewRunner(job.OSExecutor{}, 4) // 4 worker goroutines
//	defer r.Stop()
//
//	handle := r.Submit(job.Job{
//	    Command: "echo hello",
//	    OnDone: func(res job.Result) {
//	        fmt.Printf("stdout: %s\n", res.Stdout)
//	    },
//	})
//	handle.Wait()
//
// # Uses
//
// Powers #(shell-cmd) in format strings, run-shell, if-shell, and
// pipe-pane's external process. NOT used for pane shells — those are
// long-running and managed by package pane directly.
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
