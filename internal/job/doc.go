// Package job runs short-lived helper processes asynchronously and
// delivers their output to a callback.
//
// # Boundary
//
// A Runner manages a pool of goroutines. Submit a Job and get back a
// Handle; the Runner spawns the process, captures stdout/stderr, and
// invokes the completion callback on a worker goroutine.
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
