package socket

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// sockPath returns a short AF_UNIX path under /tmp to stay within
// the macOS 104-byte sun_path limit. Cleanup is registered via t.
func sockPath(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("/tmp", "dmuxsk")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return filepath.Join(d, "s")
}

// withShortPoll makes DialOrStart retry quickly so concurrency
// tests don't take seconds.
func withShortPoll(t *testing.T) {
	t.Helper()
	oldInterval, oldAttempts := dialPollInterval, dialPollAttempts
	dialPollInterval = 5 * time.Millisecond
	dialPollAttempts = 20
	t.Cleanup(func() {
		dialPollInterval = oldInterval
		dialPollAttempts = oldAttempts
	})
}

func TestListenAndDial(t *testing.T) {
	path := sockPath(t)
	l, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := l.Accept()
		if err == nil {
			accepted <- c
		}
	}()

	c, err := Dial(path)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if _, err := c.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}

	server := <-accepted
	defer server.Close()
	buf := make([]byte, 4)
	if _, err := io.ReadFull(server, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("got %q, want ping", buf)
	}
}

func TestListenStaleFileCleanup(t *testing.T) {
	// A leftover socket file from a crashed server must not block
	// the next Listen.
	path := sockPath(t)
	if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
		t.Fatalf("seed stale: %v", err)
	}
	l, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen should clean stale file: %v", err)
	}
	l.Close()
}

func TestListenRejectsLiveSocket(t *testing.T) {
	path := sockPath(t)
	l, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close()

	_, err = Listen(path)
	if !errors.Is(err, ErrAddressInUse) {
		t.Fatalf("got %v, want ErrAddressInUse", err)
	}
}

func TestDialOrStartNoServerCallsStart(t *testing.T) {
	withShortPoll(t)
	path := sockPath(t)

	var started atomic.Int32
	var listener net.Listener
	startServer := func() error {
		started.Add(1)
		l, err := Listen(path)
		if err != nil {
			return err
		}
		listener = l
		go func() {
			for {
				c, err := l.Accept()
				if err != nil {
					return
				}
				go io.Copy(io.Discard, c)
			}
		}()
		return nil
	}
	t.Cleanup(func() {
		if listener != nil {
			listener.Close()
		}
	})

	c, err := DialOrStart(path, startServer)
	if err != nil {
		t.Fatalf("DialOrStart: %v", err)
	}
	c.Close()

	if got := started.Load(); got != 1 {
		t.Fatalf("started=%d, want 1", got)
	}
}

func TestDialOrStartServerAlreadyRunningSkipsStart(t *testing.T) {
	withShortPoll(t)
	path := sockPath(t)

	l, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go io.Copy(io.Discard, c)
		}
	}()

	var started atomic.Int32
	c, err := DialOrStart(path, func() error {
		started.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("DialOrStart: %v", err)
	}
	c.Close()

	if got := started.Load(); got != 0 {
		t.Fatalf("started=%d, want 0 (server was already running)", got)
	}
}

func TestDialOrStartConcurrentStartsOnce(t *testing.T) {
	withShortPoll(t)
	path := sockPath(t)

	var started atomic.Int32
	var l net.Listener
	var listenMu sync.Mutex
	startServer := func() error {
		started.Add(1)
		listenMu.Lock()
		defer listenMu.Unlock()
		got, err := Listen(path)
		if err != nil {
			return err
		}
		l = got
		go func() {
			for {
				c, err := got.Accept()
				if err != nil {
					return
				}
				go io.Copy(io.Discard, c)
			}
		}()
		return nil
	}
	t.Cleanup(func() {
		listenMu.Lock()
		defer listenMu.Unlock()
		if l != nil {
			l.Close()
		}
	})

	const N = 10
	var wg sync.WaitGroup
	conns := make([]net.Conn, N)
	errs := make([]error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			conns[i], errs[i] = DialOrStart(path, startServer)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
		conns[i].Close()
	}

	// On Unix, flock serializes the racers and only the first gets
	// to call startServer. On Windows the lock is a no-op; duplicate
	// starts are possible, but they must all fail harmlessly and
	// all callers still get a connection. Assert the strong
	// invariant on Unix and a looser one otherwise.
	got := started.Load()
	if got < 1 {
		t.Fatalf("started=%d, want >=1", got)
	}
	if lockIsCrossProcess && got != 1 {
		t.Fatalf("started=%d, want 1 under cross-process lock", got)
	}
}

func TestDialOrStartPropagatesStartError(t *testing.T) {
	withShortPoll(t)
	path := sockPath(t)

	wantErr := errors.New("boom")
	_, err := DialOrStart(path, func() error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("got %v, want wrapping %v", err, wantErr)
	}
}

func TestDialOrStartTimeoutWhenServerNeverComesUp(t *testing.T) {
	withShortPoll(t)
	path := sockPath(t)

	// startServer returns nil but never starts a listener.
	_, err := DialOrStart(path, func() error { return nil })
	if !errors.Is(err, ErrServerStartTimeout) {
		t.Fatalf("got %v, want ErrServerStartTimeout", err)
	}
}
