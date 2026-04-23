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
	"github.com/dhamidi/dmux/internal/vt"
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

	// ErrNoVT is returned by Snapshot and Cursor when the pane was
	// opened without a vt.Runtime (Config.VT == nil). Callers that
	// need structured grid state must provide a runtime at Open
	// time; callers that only need raw bytes use Bytes().
	ErrNoVT = errors.New("pane: no vt runtime")
)

// Op describes what the package was doing when an error arose.
// Carried on PaneError so callers can log or dispatch on the failing
// step without parsing Error().
type Op string

const (
	OpOpen       Op = "open"
	OpWrite      Op = "write"
	OpResize     Op = "resize"
	OpSignal     Op = "signal"
	OpClose      Op = "close"
	OpSnapshot   Op = "snapshot"
	OpCursor     Op = "cursor"
	OpFormat     Op = "format"
	OpPlacements Op = "placements"
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
//
// VT is optional. When nil (M1 skeleton), Feed is a no-op and
// Snapshot/Cursor return ErrNoVT — the server falls back to its
// raw-bytes passthrough path. When non-nil, each pty chunk is fed
// into a per-pane vt.Terminal before being forwarded on bytesCh,
// and Snapshot/Cursor return live grid state.
type Config struct {
	Argv []string     // resolved argv; Argv[0] is the executable path
	Cwd  string       // cwd for the child
	Env  []string     // merged environment
	Cols int          // initial cols (>0)
	Rows int          // initial rows (>0)
	VT   *vt.Runtime  // optional; when set the pane owns a vt.Terminal
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
//
// vt access is serialized through vtMu. The contract is stricter
// than the vt package's doc.go assumes: readLoop (goroutine A) calls
// Feed, while the server goroutine (goroutine B) can call Snapshot,
// Cursor, Resize, and Close. A single mutex is the simplest way to
// keep vt.Terminal's single-goroutine rule honest across those
// callers without a dedicated control-channel goroutine.
//
// Multiple consumers watch a pane via Subscribe: each Subscription
// carries a coalescing dirty-signal channel that fires whenever new
// bytes have been fed to the vt. subMu guards the subscriber set.
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

	vtMu sync.Mutex
	vt   *vt.Terminal // nil when Config.VT was nil

	subMu   sync.Mutex
	subs    map[*subscriber]struct{}
	subsDone bool // readLoop exited; new subscribers get a closed channel
}

// subscriber is one dirty-signal slot. ch is buffered cap=1 and
// non-blocking sends coalesce: a pending signal means the consumer
// has not yet observed the previous feed, so skipping a send is
// correct.
type subscriber struct {
	ch chan struct{}
}

// Subscription wakes a consumer whenever the pane becomes dirty.
// Dirty = new bytes have been Fed to the vt.Terminal since the
// subscriber last saw the signal. Consumers re-read via Format /
// Cursor / Placements — the signal is coalescing (multiple feeds
// between wakes fold into a single signal).
//
// The returned channel fires once immediately so a new subscriber
// can do an initial render without waiting for the next feed. When
// the pane's readLoop exits (child gone / Close), every outstanding
// subscription's channel is closed, letting for-range loops unblock.
type Subscription struct {
	Ch    <-chan struct{}
	Close func() // idempotent; removes this subscriber
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
		subs:     make(map[*subscriber]struct{}),
	}

	if cfg.VT != nil {
		term, err := cfg.VT.NewTerminal(pCtx, cfg.Cols, cfg.Rows)
		if err != nil {
			cancel()
			_ = p.Close()
			return nil, paneErr(OpOpen, nil, err, "vt terminal")
		}
		pane.vt = term
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

// Resize forwards to pty.Resize. Subscribers are signaled after a
// successful resize so each client's pump re-paints against the new
// grid dimensions; without this a window-size negotiation update would
// not show on already-attached clients until the next pty byte landed.
//
// TODO(m1:pane-vt): in the full design (doc.go) this request goes
// on controlCh so vt.Resize runs before pty.Resize. With no vt in
// M1, direct passthrough is the correct ordering.
func (p *Pane) Resize(cols, rows int) error {
	if p.closed.Load() {
		return paneErr(OpResize, ErrClosed, nil, "")
	}
	if p.vt != nil {
		p.vtMu.Lock()
		err := p.vt.Resize(cols, rows)
		p.vtMu.Unlock()
		if err != nil {
			return paneErr(OpResize, nil, err, "vt resize cols=%d rows=%d", cols, rows)
		}
	}
	if err := p.pty.Resize(cols, rows); err != nil {
		return paneErr(OpResize, nil, err, "cols=%d rows=%d", cols, rows)
	}
	p.signalDirty()
	return nil
}

// Bytes returns the channel carrying raw pty output. Closed when
// the pty reader goroutine exits (child closed its end or Close
// ran).
//
// The channel is a best-effort tap: readLoop sends non-blocking, so
// chunks are dropped when the buffer (cap 32) is full. This keeps
// readLoop making forward progress when no one drains — the server
// uses Subscribe() for dirty signals and does not read here, so a
// blocking send would wedge the pipeline once the buffer saturated.
// Tests that care about every byte must drain promptly; under
// normal scheduling the buffer absorbs ordinary bursts without
// drops.
func (p *Pane) Bytes() <-chan []byte {
	return p.bytesCh
}

// Subscribe adds a dirty-signal subscriber. Safe to call
// concurrently with Feed and Close. The returned channel is fired
// once before return so the caller can render immediately.
//
// Close on the Subscription is idempotent — calling it twice, or
// calling it after readLoop has already closed the channel, is a
// no-op.
func (p *Pane) Subscribe() Subscription {
	s := &subscriber{ch: make(chan struct{}, 1)}

	p.subMu.Lock()
	if p.subsDone {
		// readLoop has already exited. Hand back a closed channel so
		// consumers observe "no more signals, we're done" immediately.
		p.subMu.Unlock()
		close(s.ch)
		return Subscription{
			Ch:    s.ch,
			Close: func() {},
		}
	}
	p.subs[s] = struct{}{}
	p.subMu.Unlock()

	// Prime the channel so the new subscriber renders once without
	// needing to wait for the next feed. Non-blocking send: slot is
	// empty by construction.
	select {
	case s.ch <- struct{}{}:
	default:
	}

	var closeOnce sync.Once
	return Subscription{
		Ch: s.ch,
		Close: func() {
			closeOnce.Do(func() {
				p.subMu.Lock()
				if _, ok := p.subs[s]; ok {
					delete(p.subs, s)
					close(s.ch)
				}
				p.subMu.Unlock()
			})
		},
	}
}

// signalDirty fans a wake-up signal out to every subscriber. Called
// by readLoop after each successful Feed. Non-blocking sends coalesce:
// a subscriber with an unread signal keeps the one it already has.
func (p *Pane) signalDirty() {
	p.subMu.Lock()
	for s := range p.subs {
		select {
		case s.ch <- struct{}{}:
		default:
		}
	}
	p.subMu.Unlock()
}

// closeSubscribers marks subscriptions done and closes every
// subscriber's channel so for-range consumers unblock. Called once
// from readLoop on exit.
func (p *Pane) closeSubscribers() {
	p.subMu.Lock()
	p.subsDone = true
	for s := range p.subs {
		close(s.ch)
		delete(p.subs, s)
	}
	p.subMu.Unlock()
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

		if p.vt != nil {
			p.vtMu.Lock()
			_ = p.vt.Close()
			p.vtMu.Unlock()
		}
	})
	return p.closeErr
}

// Snapshot returns a reified view of the terminal's live screen. Safe
// to call from any goroutine; serialized against readLoop's Feed via
// vtMu so the wasm module sees one caller at a time. Returns ErrNoVT
// when the pane was opened without a vt.Runtime.
func (p *Pane) Snapshot() (vt.Grid, error) {
	if p.vt == nil {
		return vt.Grid{}, paneErr(OpSnapshot, ErrNoVT, nil, "")
	}
	p.vtMu.Lock()
	defer p.vtMu.Unlock()
	g, err := p.vt.Snapshot()
	if err != nil {
		return vt.Grid{}, paneErr(OpSnapshot, nil, err, "")
	}
	return g, nil
}

// Cursor returns the cursor position of the live screen. Same
// goroutine-safety and ErrNoVT contract as Snapshot.
func (p *Pane) Cursor() (vt.Cursor, error) {
	if p.vt == nil {
		return vt.Cursor{}, paneErr(OpCursor, ErrNoVT, nil, "")
	}
	p.vtMu.Lock()
	defer p.vtMu.Unlock()
	c, err := p.vt.Cursor()
	if err != nil {
		return vt.Cursor{}, paneErr(OpCursor, nil, err, "")
	}
	return c, nil
}

// Format renders the terminal's current screen into VT sequence bytes
// via libghostty-vt's formatter. Safe to call from any goroutine;
// serialized against readLoop's Feed via vtMu. Returns ErrNoVT when
// the pane was opened without a vt.Runtime.
func (p *Pane) Format(opts vt.FormatOptions) ([]byte, error) {
	if p.vt == nil {
		return nil, paneErr(OpFormat, ErrNoVT, nil, "")
	}
	p.vtMu.Lock()
	defer p.vtMu.Unlock()
	out, err := p.vt.Format(opts)
	if err != nil {
		return nil, paneErr(OpFormat, nil, err, "")
	}
	return out, nil
}

// Placements returns the kitty graphics placements captured from the
// pty stream since the last call. The vt.Terminal's parser owns the
// state; this method only adapts the lock and the error type.
//
// Returns ErrNoVT when the pane was opened without a vt.Runtime, and
// nil/nil when the parser has nothing pending.
func (p *Pane) Placements() ([]vt.Placement, error) {
	if p.vt == nil {
		return nil, paneErr(OpPlacements, ErrNoVT, nil, "")
	}
	p.vtMu.Lock()
	defer p.vtMu.Unlock()
	out, err := p.vt.Placements()
	if err != nil {
		return nil, paneErr(OpPlacements, nil, err, "")
	}
	return out, nil
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
	defer p.closeSubscribers()

	buf := make([]byte, readBufSize)
	for {
		n, err := p.pty.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if p.vt != nil {
				p.vtMu.Lock()
				feedErr := p.vt.Feed(chunk)
				p.vtMu.Unlock()
				if feedErr != nil && !errors.Is(feedErr, vt.ErrClosed) {
					// Feed only fails on wasm-side problems. Drop the
					// chunk from the vt pipeline but still forward the
					// raw bytes so the skeleton output path keeps
					// working. The caller sees the error next time they
					// call Snapshot.
					_ = feedErr
				}
			}
			// Signal subscribers that the grid is dirty. Done before
			// the bytesCh send so a subscribed consumer can observe
			// the signal without needing to also drain bytesCh.
			p.signalDirty()
			// Non-blocking tap send: drop chunks when the buffer
			// (cap 32) is full rather than wedging readLoop. The
			// server does not drain Bytes() — it uses Subscribe —
			// so a blocking send here would freeze every downstream
			// Feed/signalDirty after the first burst of 32 chunks,
			// which presents to the user as "keypresses stop
			// echoing" because shell output never advances into vt.
			// See Bytes() doc for the tap contract.
			select {
			case p.bytesCh <- chunk:
			case <-p.ctx.Done():
				return
			default:
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
