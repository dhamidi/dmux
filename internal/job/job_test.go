package job_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/job"
)

// fakeExecutor is a test double that returns canned output without
// spawning any real OS processes.
type fakeExecutor struct {
	mu      sync.Mutex
	calls   []fakeCall
	handler func(name string, args []string) ([]byte, []byte, error)
}

type fakeCall struct {
	name string
	args []string
}

func (f *fakeExecutor) Run(_ context.Context, name string, args []string) ([]byte, []byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fakeCall{name: name, args: args})
	h := f.handler
	f.mu.Unlock()
	if h != nil {
		return h(name, args)
	}
	return nil, nil, nil
}

func (f *fakeExecutor) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// newFake returns a fakeExecutor whose handler returns the given stdout,
// stderr, and err for every call.
func newFake(stdout, stderr []byte, err error) *fakeExecutor {
	return &fakeExecutor{
		handler: func(string, []string) ([]byte, []byte, error) {
			return stdout, stderr, err
		},
	}
}

// TestCallbackReceivesStdout verifies that OnDone is called with the
// stdout captured from the executor.
func TestCallbackReceivesStdout(t *testing.T) {
	t.Parallel()
	want := []byte("hello\n")
	exec := newFake(want, nil, nil)

	r := job.NewRunner(exec, 1)
	defer r.Stop()

	var got []byte
	done := make(chan struct{})
	r.Submit(job.Job{
		Command: "echo hello",
		OnDone: func(res job.Result) {
			got = res.Stdout
			close(done)
		},
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("OnDone not called within timeout")
	}
	if string(got) != string(want) {
		t.Errorf("stdout: got %q, want %q", got, want)
	}
}

// TestCallbackReceivesStderr verifies that OnDone receives stderr output.
func TestCallbackReceivesStderr(t *testing.T) {
	t.Parallel()
	want := []byte("oops\n")
	exec := newFake(nil, want, nil)

	r := job.NewRunner(exec, 1)
	defer r.Stop()

	var got []byte
	done := make(chan struct{})
	r.Submit(job.Job{
		Command: "cmd arg",
		OnDone: func(res job.Result) {
			got = res.Stderr
			close(done)
		},
	})
	<-done
	if string(got) != string(want) {
		t.Errorf("stderr: got %q, want %q", got, want)
	}
}

// TestErrorPropagation verifies that errors returned by the Executor are
// forwarded to OnDone.
func TestErrorPropagation(t *testing.T) {
	t.Parallel()
	want := errors.New("exit status 1")
	exec := newFake(nil, nil, want)

	r := job.NewRunner(exec, 1)
	defer r.Stop()

	var got error
	done := make(chan struct{})
	r.Submit(job.Job{
		Command: "fail",
		OnDone: func(res job.Result) {
			got = res.Err
			close(done)
		},
	})
	<-done
	if got != want {
		t.Errorf("err: got %v, want %v", got, want)
	}
}

// TestHandleWait verifies that Handle.Wait returns after the job completes.
func TestHandleWait(t *testing.T) {
	t.Parallel()
	exec := newFake([]byte("ok"), nil, nil)

	r := job.NewRunner(exec, 1)
	defer r.Stop()

	called := false
	h := r.Submit(job.Job{
		Command: "true",
		OnDone:  func(job.Result) { called = true },
	})
	h.Wait()
	if !called {
		t.Error("OnDone was not called before Wait returned")
	}
}

// TestConcurrencyLimit verifies that no more than workers goroutines run
// the executor simultaneously.
func TestConcurrencyLimit(t *testing.T) {
	t.Parallel()
	const workers = 3
	const jobs = 12

	var (
		active    int64
		maxActive int64
		mu        sync.Mutex
		gate      = make(chan struct{}) // released to unblock workers
	)

	exec := &fakeExecutor{
		handler: func(string, []string) ([]byte, []byte, error) {
			cur := atomic.AddInt64(&active, 1)
			mu.Lock()
			if cur > maxActive {
				maxActive = cur
			}
			mu.Unlock()
			<-gate
			atomic.AddInt64(&active, -1)
			return nil, nil, nil
		},
	}

	r := job.NewRunner(exec, workers)
	defer r.Stop()

	handles := make([]*job.Handle, jobs)
	for i := range handles {
		handles[i] = r.Submit(job.Job{
			Command: "cmd",
			OnDone:  func(job.Result) {},
		})
	}

	// Let workers fill up, then release them all.
	time.Sleep(50 * time.Millisecond)
	close(gate)

	for _, h := range handles {
		h.Wait()
	}

	mu.Lock()
	got := maxActive
	mu.Unlock()

	if got > workers {
		t.Errorf("max concurrent executions: got %d, want <= %d", got, workers)
	}
	if exec.CallCount() != jobs {
		t.Errorf("call count: got %d, want %d", exec.CallCount(), jobs)
	}
}

// TestEmptyCommandCallsOnDone verifies that an empty command string still
// invokes OnDone without error.
func TestEmptyCommandCallsOnDone(t *testing.T) {
	t.Parallel()
	exec := newFake(nil, nil, nil)

	r := job.NewRunner(exec, 1)
	defer r.Stop()

	done := make(chan struct{})
	r.Submit(job.Job{
		Command: "",
		OnDone:  func(job.Result) { close(done) },
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("OnDone not called for empty command")
	}
	if exec.CallCount() != 0 {
		t.Errorf("executor should not be called for empty command, got %d calls", exec.CallCount())
	}
}
