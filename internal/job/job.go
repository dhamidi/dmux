package job

import (
	"context"
	"strings"
	"time"
)

// Job describes a single unit of work: a command to run and a callback
// to invoke when it finishes.
type Job struct {
	// Command is a shell-style command string. It is split on whitespace
	// into a program name and arguments before being passed to the
	// [Executor].
	Command string

	// Timeout limits how long the process may run. Zero means no timeout.
	Timeout time.Duration

	// OnDone is called on a worker goroutine when the command finishes.
	// It must not be nil.
	OnDone func(Result)
}

// Result carries the output and exit status of a completed [Job].
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	Err      error
}

// Handle is returned by [Runner.Submit]. It can be used to wait for a job
// or to cancel it before it starts executing.
type Handle struct {
	done chan struct{}
}

// Wait blocks until the job associated with this Handle has completed.
func (h *Handle) Wait() {
	<-h.done
}

// Runner manages a bounded pool of worker goroutines. Each worker picks
// jobs off a shared queue, runs them via the injected [Executor], and
// dispatches the [Result] to the job's OnDone callback.
type Runner struct {
	exec    Executor
	queue   chan work
	workers int
}

type work struct {
	job    Job
	handle *Handle
}

// NewRunner creates a Runner that uses exec for process spawning and
// maintains workers goroutines to execute submitted jobs concurrently.
// Callers that want the default OS behaviour should pass [OSExecutor]{}.
func NewRunner(exec Executor, workers int) *Runner {
	if workers < 1 {
		workers = 1
	}
	r := &Runner{
		exec:    exec,
		queue:   make(chan work, workers*4),
		workers: workers,
	}
	for i := 0; i < workers; i++ {
		go r.loop()
	}
	return r
}

// Submit enqueues a job for execution and returns a Handle that can be
// used to wait for it. Submit returns immediately; work is performed
// asynchronously on a worker goroutine.
func (r *Runner) Submit(j Job) *Handle {
	h := &Handle{done: make(chan struct{})}
	r.queue <- work{job: j, handle: h}
	return h
}

// Stop shuts down the worker pool. It closes the queue so that all
// workers drain remaining items and exit. Stop does not wait for
// in-flight jobs to complete; callers that need that guarantee should
// Wait on outstanding Handles before calling Stop.
func (r *Runner) Stop() {
	close(r.queue)
}

func (r *Runner) loop() {
	for w := range r.queue {
		r.run(w)
	}
}

func (r *Runner) run(w work) {
	j := w.job
	defer close(w.handle.done)

	ctx := context.Background()
	var cancel context.CancelFunc
	if j.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, j.Timeout)
		defer cancel()
	}

	parts := splitCommand(j.Command)
	if len(parts) == 0 {
		j.OnDone(Result{Err: nil})
		return
	}

	stdout, stderr, err := r.exec.Run(ctx, parts[0], parts[1:])

	res := Result{
		Stdout: stdout,
		Stderr: stderr,
		Err:    err,
	}
	if err != nil {
		if exitErr, ok := err.(interface{ ExitCode() int }); ok {
			res.ExitCode = exitErr.ExitCode()
		}
	}

	j.OnDone(res)
}

// splitCommand splits a command string on whitespace into a slice of
// tokens. It does not handle quoting; callers that need shell semantics
// should use "sh", "-c", command.
func splitCommand(cmd string) []string {
	return strings.Fields(cmd)
}
