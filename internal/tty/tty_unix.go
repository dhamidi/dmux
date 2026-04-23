//go:build unix

package tty

import (
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"golang.org/x/sys/unix"
)

// TTY is a handle on the client's real terminal device.
//
// The zero value is not useful; obtain one via Open. All methods
// are safe for concurrent use; the common layout is one reader
// goroutine parked in Read, a writer goroutine calling Write, and
// a third goroutine consuming the Resize channel.
type TTY struct {
	in  *os.File // stdin, the device we read keystrokes from
	out *os.File // stdout, the device we write rendered output to

	// termios state. savedTermios holds the pre-Raw settings so
	// Restore can undo; rawApplied reports whether Raw has been
	// called since the last Restore (or since Open).
	mu           sync.Mutex
	savedTermios *unix.Termios
	rawApplied   bool

	// SIGWINCH plumbing. sigCh receives the OS signal; resizeCh is
	// the public event channel. A dedicated goroutine translates
	// one to the other and exits when stopCh closes.
	sigCh    chan os.Signal
	resizeCh chan ResizeEvent
	stopCh   chan struct{}
	stopOnce sync.Once

	closed atomic.Bool
	once   sync.Once
}

// Open wraps the caller's stdin/stdout as a TTY. Both fds must
// refer to a terminal device; if either does not, Open returns an
// error wrapping ErrNotATerminal.
//
// Open does not change the terminal mode. Call Raw explicitly when
// ready to take over input; Close (or Restore) puts the saved
// settings back.
func Open(stdin, stdout *os.File) (*TTY, error) {
	if stdin == nil || stdout == nil {
		return nil, ttyErr(OpOpen, ErrNotATerminal, nil, "nil fd")
	}
	// IoctlGetTermios on a non-tty fails with ENOTTY. Using it as
	// the probe avoids a separate isatty helper and exercises the
	// same syscall we'll use again in Raw.
	if _, err := unix.IoctlGetTermios(int(stdin.Fd()), ioctlGetTermios); err != nil {
		return nil, ttyErr(OpOpen, ErrNotATerminal, err, "stdin")
	}
	if _, err := unix.IoctlGetTermios(int(stdout.Fd()), ioctlGetTermios); err != nil {
		return nil, ttyErr(OpOpen, ErrNotATerminal, err, "stdout")
	}

	t := &TTY{
		in:       stdin,
		out:      stdout,
		sigCh:    make(chan os.Signal, 1),
		resizeCh: make(chan ResizeEvent, 1),
		stopCh:   make(chan struct{}),
	}
	signal.Notify(t.sigCh, syscall.SIGWINCH)
	go t.watchResize()
	return t, nil
}

// Raw puts stdin into raw mode. The prior termios is saved; a
// matching Restore (or Close) returns the terminal to the state it
// was in when Raw was called.
//
// Repeated Raw calls without an intervening Restore are a no-op
// that still succeeds — the saved termios is the one from the
// first Raw, which is the expected Restore target.
func (t *TTY) Raw() error {
	if t.closed.Load() {
		return ErrClosed
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.rawApplied {
		return nil
	}
	cur, err := unix.IoctlGetTermios(int(t.in.Fd()), ioctlGetTermios)
	if err != nil {
		return ttyErr(OpRaw, ErrRaw, err, "get-termios")
	}
	// Copy and mutate. We explicitly mirror cfmakeraw so we know
	// which bits we set rather than relying on libc semantics.
	raw := *cur
	raw.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	raw.Oflag &^= unix.OPOST
	raw.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	raw.Cflag &^= unix.CSIZE | unix.PARENB
	raw.Cflag |= unix.CS8
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(int(t.in.Fd()), ioctlSetTermios, &raw); err != nil {
		return ttyErr(OpRaw, ErrRaw, err, "set-termios")
	}
	saved := *cur
	t.savedTermios = &saved
	t.rawApplied = true
	return nil
}

// Restore puts the terminal back into the settings captured by the
// most recent Raw call. Idempotent: calling Restore without a prior
// Raw, or calling it twice, is not an error.
func (t *TTY) Restore() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.restoreLocked()
}

func (t *TTY) restoreLocked() error {
	if !t.rawApplied || t.savedTermios == nil {
		return nil
	}
	if err := unix.IoctlSetTermios(int(t.in.Fd()), ioctlSetTermios, t.savedTermios); err != nil {
		return ttyErr(OpRestore, ErrRestore, err, "set-termios")
	}
	t.rawApplied = false
	t.savedTermios = nil
	return nil
}

// Size returns the current cell dimensions of the terminal. It
// queries TIOCGWINSZ on the stdout fd, which is what the server's
// renderer will draw into.
func (t *TTY) Size() (cols, rows int, err error) {
	if t.closed.Load() {
		return 0, 0, ErrClosed
	}
	ws, err := unix.IoctlGetWinsize(int(t.out.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return 0, 0, ttyErr(OpSize, ErrGetSize, err, "")
	}
	return int(ws.Col), int(ws.Row), nil
}

// Read reads bytes typed by the user. In M1 these flow straight
// through to the server as Input frame payload without
// interpretation. Blocks until at least one byte is available or
// the fd is closed.
func (t *TTY) Read(p []byte) (int, error) {
	if t.closed.Load() {
		return 0, ErrClosed
	}
	return t.in.Read(p)
}

// Write sends bytes to the user's terminal. These are typically
// the bytes produced by termout (rendered pane + status line).
func (t *TTY) Write(p []byte) (int, error) {
	if t.closed.Load() {
		return 0, ErrClosed
	}
	return t.out.Write(p)
}

// Resize returns a channel that receives ResizeEvent values when
// the terminal window is resized. The channel has capacity 1 and
// coalescing semantics: if an event is pending and a new resize
// arrives, the newest size replaces it. Callers that care about
// every transition must drain promptly; callers that only want the
// final size can sample at their convenience.
//
// The channel is closed by Close. Consumers should therefore treat
// a closed channel as "TTY is shutting down."
func (t *TTY) Resize() <-chan ResizeEvent {
	return t.resizeCh
}

// EnableModes is a no-op stub for M1. When internal/termcaps lands
// it will take a termcaps.Profile and emit the appropriate CSI
// enable sequences (SGR mouse, bracketed paste, focus events,
// KKP); Close will emit the matching disable sequences. See
// TODO(m1:tty-enablemodes).
//
// TODO(m1:tty-enablemodes): grow an argument of type
// termcaps.Profile and write the prologue defined in
// internal/termcaps.Features. The method is exposed now so callers
// in internal/client can be wired in before termcaps is real.
func (t *TTY) EnableModes() error {
	if t.closed.Load() {
		return ErrClosed
	}
	return nil
}

// Close restores the terminal to its pre-Raw settings, stops the
// SIGWINCH handler, and closes the Resize channel. Idempotent.
// The underlying stdin/stdout files are left open — they belong to
// the caller.
func (t *TTY) Close() error {
	var restoreErr error
	t.once.Do(func() {
		t.closed.Store(true)
		t.mu.Lock()
		restoreErr = t.restoreLocked()
		t.mu.Unlock()
		signal.Stop(t.sigCh)
		t.stopOnce.Do(func() { close(t.stopCh) })
	})
	return restoreErr
}

// watchResize translates SIGWINCH signals into ResizeEvent values
// on the public resizeCh. Non-blocking send with coalescing: if a
// previous event is still unread, drop it and publish the newest
// size instead.
func (t *TTY) watchResize() {
	defer close(t.resizeCh)
	for {
		select {
		case <-t.stopCh:
			return
		case _, ok := <-t.sigCh:
			if !ok {
				return
			}
			cols, rows, err := t.Size()
			if err != nil {
				// Can't do much here; the fd may have gone away.
				// The next Size() call from the user will surface
				// the error directly. Drop this event.
				continue
			}
			ev := ResizeEvent{Cols: cols, Rows: rows}
			select {
			case t.resizeCh <- ev:
			default:
				// Drop the stale pending event, then publish the
				// fresh one. If a reader steals the slot between
				// the receive and the send, that's fine — they
				// either got the old or the new, and either way
				// no event is lost past the latest.
				select {
				case <-t.resizeCh:
				default:
				}
				select {
				case t.resizeCh <- ev:
				default:
				}
			}
		}
	}
}
