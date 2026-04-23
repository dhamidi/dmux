package pane

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/dhamidi/dmux/internal/pty"
)

// Current scope (M1 walking skeleton):
//
//   - Open (pty.Spawn), Write (raw SendBytes passthrough), Resize,
//     Bytes channel, Exited channel, Signal, Close: REAL.
//   - vt.Terminal integration, snapshot pushing, render ticker,
//     dirty-tracking, KeyEncoder, mouse, capability mediation,
//     attached-profiles snapshot, controlCh: STUBBED (not present).
//     These arrive with internal/vt and the M1 path described in
//     internal/pane/doc.go. Search for TODO(m1:pane-vt) for the
//     introduction point.
//
// When vt lands, the public API grows SendKey, SendMouse, Snapshot,
// Snapshots, AttachedClients; Write stays as the M1 bypass and is
// removed in M2-1 per docs/m1.md. Resize will move onto controlCh
// so vt.Resize runs before pty.Resize (see doc.go).

// Sentinel errors. Callers use errors.Is to dispatch on category.
// The concrete error returned by this package is usually a
// *PaneError wrapping one of these, so errors.As can also pull out
// the operation that failed.
var (
	// ErrClosed is returned by Write, Resize, and Signal when Close
	// has already run.
	ErrClosed = errors.New("pane: closed")

	// ErrSpawn is returned by Open when pty.Spawn fails. The
	// underlying pty sentinel (ErrOpenPty, ErrStartProcess, etc.)
	// remains reachable via errors.Is.
	ErrSpawn = errors.New("pane: spawn failed")
)

// Op describes what the package was doing when an error arose.
// Carried on PaneError so callers can log or dispatch on the failing
// step without parsing Error().
type Op string

const (
	OpOpen   Op = "open"
	OpWrite  Op = "write"
	OpResize Op = "resize"
	OpSignal Op = "signal"
	OpClose  Op = "close"
)

// PaneError is the concrete error type returned by this package.
// It carries a category Sentinel (ErrClosed, ErrSpawn) and a wrapped
// Err chain so both classification via errors.Is and structured
// inspection via errors.As work:
//
//	var pe *pane.PaneError
//	if errors.As(err, &pe) {
//	    // pe.Op, pe.Detail are available
//	}
//	if errors.Is(err, pane.ErrClosed) { ... }
//	if errors.Is(err, pty.ErrStartProcess) { ... } // survives Open failures
//
// Unwrap returns Err so the underlying pty chain remains reachable
// to errors.Is/errors.As; Is also answers true for Sentinel so the
// pane's own classification works independently of the cause.
type PaneError struct {
	Op       Op
	Sentinel error // one of the package sentinels, or nil
	Detail   string
	Err      error // underlying cause, or nil
}

func (e *PaneError) Error() string {
	var b strings.Builder
	b.WriteString("pane: ")
	b.WriteString(string(e.Op))
	if e.Sentinel != nil {
		b.WriteString(": ")
		// Strip the "pane: " prefix from the sentinel text so the
		// composed message does not repeat it.
		b.WriteString(strings.TrimPrefix(e.Sentinel.Error(), "pane: "))
	}
	if e.Detail != "" {
		b.WriteString(": ")
		b.WriteString(e.Detail)
	}
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

// Unwrap returns the underlying cause so errors.Is and errors.As
// traverse the full chain. The package sentinel is reported via Is
// directly.
func (e *PaneError) Unwrap() error { return e.Err }

// Is lets errors.Is match the package sentinel even when Err carries
// an unrelated chain (e.g. Open: Sentinel=ErrSpawn, Err=pty's chain
// containing ErrStartProcess — both must match).
func (e *PaneError) Is(target error) bool {
	return e.Sentinel != nil && e.Sentinel == target
}

// paneErr constructs a *PaneError.
func paneErr(op Op, sentinel, cause error, format string, args ...any) error {
	var detail string
	if format != "" {
		detail = fmt.Sprintf(format, args...)
	}
	return &PaneError{
		Op:       op,
		Sentinel: sentinel,
		Detail:   detail,
		Err:      cause,
	}
}

// Config describes the child process for a pane. The caller has
// already resolved Argv (Argv[0] is the executable path, not a
// name to look up), merged Env from the appropriate option scopes,
// picked Cwd, and chosen initial dimensions.
type Config struct {
	Argv []string // resolved argv; Argv[0] is the executable path
	Cwd  string   // cwd for the child
	Env  []string // merged environment
	Cols int      // initial cols (>0)
	Rows int      // initial rows (>0)
}

// bytesChanBuffer is the slot count on the pty-output channel. Each
// slot holds one Read's worth of bytes (up to 4KiB), so the total
// queueable output is ~128KiB before backpressure kicks in. That is
// enough to absorb a burst while the server is busy without starving
// the reader on quiescent panes.
const bytesChanBuffer = 32

// readBufSize is the per-Read buffer allocated by the reader
// goroutine. 4KiB matches typical pty output block sizes on Linux
// and Darwin.
const readBufSize = 4096

// Pane is one pty attached to one child process, plus two helper
// goroutines (pty reader + exit waiter). The server sends bytes in
// via Write, reads pty output off Bytes(), and receives the child's
// exit status on Exited().
//
// All methods are goroutine-safe. Close is idempotent.
type Pane struct {
	pty *pty.PTY

	ctx    context.Context
	cancel context.CancelFunc

	bytesCh  chan []byte
	exitedCh chan pty.ExitStatus

	wg sync.WaitGroup

	closeOnce sync.Once
	closeErr  error
	closed    atomic.Bool
}

// Open spawns the child under a fresh pty and starts the helper
// goroutines. Errors from pty.Spawn propagate through ErrSpawn with
// the underlying cause wrapped via %w so callers can still use
// errors.Is on pty.ErrStartProcess / pty.ErrOpenPty, and errors.As
// on *pty.SpawnError.
func Open(ctx context.Context, cfg Config) (*Pane, error) {
	p, err := pty.Spawn(ctx, pty.Config{
		Argv: cfg.Argv,
		Cwd:  cfg.Cwd,
		Env:  cfg.Env,
		Cols: cfg.Cols,
		Rows: cfg.Rows,
	})
	if err != nil {
		return nil, paneErr(OpOpen, ErrSpawn, err, "")
	}

	pCtx, cancel := context.WithCancel(ctx)
	pane := &Pane{
		pty:      p,
		ctx:      pCtx,
		cancel:   cancel,
		bytesCh:  make(chan []byte, bytesChanBuffer),
		exitedCh: make(chan pty.ExitStatus, 1),
	}

	pane.wg.Add(2)
	go pane.readLoop()
	go pane.waitLoop()

	return pane, nil
}

// Write sends raw stdin bytes to the child. This is the M1 SendBytes
// shortcut documented in docs/m1.md — bytes go straight to pty.Write
// with no parsing. M2-1 replaces this with SendKey/SendMouse/SendFocus
// through a real encoder.
func (p *Pane) Write(b []byte) (int, error) {
	if p.closed.Load() {
		return 0, paneErr(OpWrite, ErrClosed, nil, "")
	}
	n, err := p.pty.Write(b)
	if err != nil {
		return n, paneErr(OpWrite, nil, err, "")
	}
	return n, nil
}

// Resize forwards to pty.Resize.
//
// TODO(m1:pane-vt): in the full design (doc.go) this request goes
// on controlCh so vt.Resize runs before pty.Resize. With no vt in
// M1, direct passthrough is the correct ordering.
func (p *Pane) Resize(cols, rows int) error {
	if p.closed.Load() {
		return paneErr(OpResize, ErrClosed, nil, "")
	}
	if err := p.pty.Resize(cols, rows); err != nil {
		return paneErr(OpResize, nil, err, "cols=%d rows=%d", cols, rows)
	}
	return nil
}

// Bytes returns the channel carrying raw pty output. Closed when
// the pty reader goroutine exits (child closed its end or Close
// ran). Consumers must receive promptly; the channel is buffered
// but will backpressure under sustained heavy output.
func (p *Pane) Bytes() <-chan []byte {
	return p.bytesCh
}

// Exited returns a one-shot channel carrying the child's exit
// status. Fires exactly once, then closes. If pty.Wait returned an
// error (rare; usually means the child was adopted elsewhere), a
// zero ExitStatus is delivered and the channel closes — the server
// treats the channel-close as the "child is gone" signal regardless
// of the payload.
func (p *Pane) Exited() <-chan pty.ExitStatus {
	return p.exitedCh
}

// Signal forwards to pty.Signal.
func (p *Pane) Signal(sig pty.Signal) error {
	if p.closed.Load() {
		return paneErr(OpSignal, ErrClosed, nil, "")
	}
	if err := p.pty.Signal(sig); err != nil {
		return paneErr(OpSignal, nil, err, "sig=%d", sig)
	}
	return nil
}

// Close cancels the pane's context, sends SIGHUP to the child
// process group, waits for the helper goroutines to exit, and
// closes the pty. Idempotent; repeated calls return nil.
func (p *Pane) Close() error {
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		p.cancel()

		// Best-effort SIGHUP: the child may already be gone (it
		// exited on its own, or pty.Close is about to take it out
		// anyway). Errors from a dead child are expected; drop them.
		_ = p.pty.Signal(pty.SIGHUP)

		// Closing the master fd wakes the reader goroutine (Read
		// returns an error) and eventually lets Wait report the
		// child's status.
		p.closeErr = p.pty.Close()

		p.wg.Wait()
	})
	return p.closeErr
}

// readLoop is the pty reader helper. It runs a Read loop, emitting
// owned copies of each chunk on bytesCh. On Read error (child
// closed, Close ran) it returns and closes bytesCh so consumers see
// the pipe close.
//
// Per-chunk copy mirrors internal/client/client.go's stdin loop:
// the receiver owns its buffer, the next Read is free to overwrite
// the local scratch buffer.
func (p *Pane) readLoop() {
	defer p.wg.Done()
	defer close(p.bytesCh)

	buf := make([]byte, readBufSize)
	for {
		n, err := p.pty.Read(buf)
		if n > 0 {
			// TODO(m1:pane-vt): once internal/vt lands, feed buf[:n]
			// to vt.Terminal.Feed here (on the pane goroutine) and
			// push snapshots to the server rather than raw bytes.
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			select {
			case p.bytesCh <- chunk:
			case <-p.ctx.Done():
				return
			}
		}
		if err != nil {
			// EOF, ErrClosed, or a transport-level error: all mean
			// the pty is done producing output. The close(bytesCh)
			// deferred above signals that to consumers.
			_ = err // explicit: do not log; the caller decides.
			if errors.Is(err, io.EOF) || errors.Is(err, pty.ErrClosed) {
				return
			}
			return
		}
	}
}

// waitLoop is the exit-waiter helper. Wait is idempotent on the pty
// side so calling it from here is safe even if another goroutine
// also called Wait (none currently does). The result is delivered
// exactly once, then exitedCh is closed so the server's channel
// close counts as "child is gone."
func (p *Pane) waitLoop() {
	defer p.wg.Done()
	defer close(p.exitedCh)

	st, err := p.pty.Wait()
	if err != nil {
		// Deliver a zero ExitStatus on Wait failure. The server
		// treats channel-close as the signal; the payload is a
		// best-effort hint. Attaching the error here would force an
		// Exited() signature change that M1 is not ready for.
		p.exitedCh <- pty.ExitStatus{}
		return
	}
	p.exitedCh <- st
}
