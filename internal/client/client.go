package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/tty"
	"github.com/dhamidi/dmux/internal/xio"
)

// Current scope (M1 walking skeleton):
//
//   - Handshake (Identify + CommandList): REAL.
//   - Input/Output/Resize byte pump: REAL.
//   - Exit handling: REAL for all documented ExitReason values.
//   - Capability probing (DA2, KKP detection) per doc.go step 3:
//     STUBBED. Profile is whatever the caller supplies in Options.
//     See TODO(m1:client-caps) markers.
//   - Bye frame on clean detach: STUBBED. M1 only exits via
//     server-initiated Exit; Ctrl-B-d lands in M2-1. See
//     TODO(m1:client-bye).
//   - CommandResult non-Ok status handling: we log and continue.
//     M2 will route these back to the caller.

// Sentinel errors. Callers use errors.Is to dispatch on category.
// The concrete error returned by this package is usually a
// *ClientError wrapping one of these, so errors.As can also pull
// out the operation that failed.
var (
	// ErrProtocol is returned when the peer sends an unexpected
	// frame type, or a handshake response is malformed.
	ErrProtocol = errors.New("client: protocol error")

	// ErrHandshake is returned when the Identify/CommandList
	// exchange fails before the steady-state goroutines start.
	ErrHandshake = errors.New("client: handshake failed")

	// ErrLostConnection is returned when the reader hits EOF or a
	// transport error after the handshake has completed.
	ErrLostConnection = errors.New("client: lost connection")
)

// Op describes what the client was doing when an error arose.
// Carried on ClientError so callers can log or dispatch on the
// failing step without parsing Error().
type Op string

const (
	OpHandshake  Op = "handshake"
	OpReadFrame  Op = "read-frame"
	OpWriteFrame Op = "write-frame"
	OpDispatch   Op = "dispatch"
)

// ClientError is the concrete error type returned by Run. It wraps
// one of the sentinels so errors.Is still classifies the category,
// and carries the Op plus a free-form Detail for logs.
//
//	var ce *client.ClientError
//	if errors.As(err, &ce) {
//	    // ce.Op, ce.Detail are available
//	}
//	if errors.Is(err, client.ErrLostConnection) { ... }
type ClientError struct {
	Op     Op
	Detail string
	Err    error
}

func (e *ClientError) Error() string {
	var b strings.Builder
	b.WriteString("client: ")
	b.WriteString(string(e.Op))
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

func (e *ClientError) Unwrap() error { return e.Err }

// clientErr constructs a *ClientError wrapping one of the sentinels.
func clientErr(op Op, sentinel, cause error, format string, args ...any) error {
	var detail string
	if format != "" {
		detail = fmt.Sprintf(format, args...)
	}
	var underlying error
	switch {
	case sentinel != nil && cause != nil:
		underlying = fmt.Errorf("%w: %v", sentinel, cause)
	case sentinel != nil:
		underlying = sentinel
	case cause != nil:
		underlying = cause
	}
	return &ClientError{Op: op, Detail: detail, Err: underlying}
}

// Terminal is the slice of *tty.TTY the client uses. Declared as an
// interface so tests can substitute a scripted fake without opening
// a real pty.
type Terminal interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Size() (cols, rows int, err error)
	Resize() <-chan tty.ResizeEvent
}

// Options describes the client's identity and what commands to send
// at attach time.
type Options struct {
	Profile  uint8           // termcaps profile on the wire; M1: always 0 (Unknown)
	Cwd      string
	TTYName  string          // best-effort; empty string is allowed
	TermEnv  string          // value of $TERM at startup
	Env      []string        // merged environment to pass to the session
	Commands []proto.Command // sent as a single CommandList after Identify (may be empty)
}

// Result captures how the session ended.
type Result struct {
	ExitReason  proto.ExitReason
	ExitMessage string
}

// Run drives one client session to completion. It returns when:
//
//   - The server sends Exit (Result is populated, err is nil).
//   - The context is canceled (err wraps ctx.Err()).
//   - The connection errors mid-stream (err wraps the I/O error).
//
// Run does not close conn or t — the caller owns those.
//
// # Goroutine layout
//
// After the handshake completes, Run starts four goroutines, all
// derived from an internal context:
//
//   - reader: loops on xio.FrameReader.ReadFrame, dispatches
//     Output (-> t.Write), Exit (-> record + signal shutdown),
//     CommandResult/Beep (-> log and ignore), unknown (-> close
//     with ErrProtocol).
//   - writer: drains a buffered send channel, calling
//     xio.FrameWriter.WriteFrame. This is the one site that
//     touches the FrameWriter, which is explicitly not
//     concurrency-safe.
//   - resize: selects on t.Resize() and ctx.Done(); enqueues
//     *proto.Resize frames on the send channel.
//   - stdin: reads bytes from t in a loop and enqueues
//     *proto.Input frames.
//
// # Context cancellation
//
// The reader, writer, and resize goroutines are cancellable via
// ctx. The stdin goroutine is blocked in t.Read and cannot be
// safely interrupted without closing the tty (which Run does not
// own). Run therefore allows the stdin goroutine to leak on
// shutdown; the caller is expected to return from main shortly
// after Run returns, which bounds the leak to program lifetime.
//
// When ctx is canceled, Run starts shutdown but does not close
// conn — outstanding goroutines that block on conn I/O will exit
// when the caller closes conn or when the peer hangs up.
func Run(ctx context.Context, conn net.Conn, t Terminal, opts Options) (Result, error) {
	if err := handshake(conn, t, opts); err != nil {
		return Result{}, err
	}

	// Internal context: we cancel it to signal shutdown to the
	// reader, writer, and resize goroutines. The caller's ctx
	// is a parent so external cancellation propagates in.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	fr := xio.NewReader(conn)
	fw := xio.NewWriter(conn)

	// Send channel and writer goroutine. Buffered so that the
	// resize and stdin goroutines don't block on each other in
	// bursty cases; the writer pulls as fast as the socket allows.
	sendCh := make(chan proto.Frame, 16)
	var sendOnce sync.Once
	closeSend := func() { sendOnce.Do(func() { close(sendCh) }) }

	// WaitGroup covers reader, writer, resize. NOT stdin — see
	// the function doc. The stdin goroutine is allowed to leak.
	var wg sync.WaitGroup

	// Shared session-end state. The reader goroutine writes it
	// when it observes Exit or a transport error; Run reads it
	// after wg.Wait. A mutex guards the write/read because the
	// reader and the main goroutine do not otherwise synchronize.
	var (
		resultMu  sync.Mutex
		result    Result
		runErr    error
		exitClean bool
	)
	setResult := func(res Result) {
		resultMu.Lock()
		defer resultMu.Unlock()
		if !exitClean && runErr == nil {
			result = res
			exitClean = true
		}
	}
	setErr := func(err error) {
		resultMu.Lock()
		defer resultMu.Unlock()
		if !exitClean && runErr == nil {
			runErr = err
		}
	}

	// Writer goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for f := range sendCh {
			if err := fw.WriteFrame(f); err != nil {
				setErr(clientErr(OpWriteFrame, ErrLostConnection, err, ""))
				// Drain remaining frames without writing so the
				// senders don't block, but keep the first error.
				for range sendCh {
				}
				return
			}
		}
	}()

	// Reader goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()       // wake the resize goroutine
		defer closeSend()    // wake the writer goroutine
		for {
			f, err := fr.ReadFrame()
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					setErr(clientErr(OpReadFrame, ErrLostConnection, err, "eof"))
				} else if isCanceled(runCtx) {
					// Context canceled; connection error is a
					// consequence, not the cause. Report ctx.Err.
					setErr(fmt.Errorf("client: %w", runCtx.Err()))
				} else {
					setErr(clientErr(OpReadFrame, ErrLostConnection, err, ""))
				}
				return
			}
			done, dispatchErr := dispatch(f, t, setResult)
			if dispatchErr != nil {
				setErr(dispatchErr)
				return
			}
			if done {
				return
			}
		}
	}()

	// Resize goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		resizeCh := t.Resize()
		for {
			select {
			case <-runCtx.Done():
				return
			case ev, ok := <-resizeCh:
				if !ok {
					return
				}
				f := &proto.Resize{Cols: uint32(ev.Cols), Rows: uint32(ev.Rows)}
				select {
				case <-runCtx.Done():
					return
				case sendCh <- f:
				}
			}
		}
	}()

	// Stdin goroutine. INTENTIONALLY not part of wg; it is
	// blocked in t.Read and we have no safe way to interrupt it
	// here. See function doc.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := t.Read(buf)
			if n > 0 {
				// Copy: Input's MarshalBinary returns Data by
				// reference, and buf is reused on the next Read.
				payload := make([]byte, n)
				copy(payload, buf[:n])
				f := &proto.Input{Data: payload}
				select {
				case <-runCtx.Done():
					return
				case sendCh <- f:
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// A goroutine waits for all three tracked goroutines to exit
	// so the main goroutine can select between "session ended" and
	// "caller canceled ctx."
	allDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(allDone)
	}()

	select {
	case <-allDone:
		// Reader observed Exit, EOF, or a transport error; its
		// defers have already canceled runCtx and closed sendCh.
	case <-ctx.Done():
		// Caller canceled. The reader is parked in ReadFrame; we
		// can't safely unblock it without closing conn (not ours).
		// Record the cancel reason and wait for the reader to exit
		// on its own (next frame, peer hangup, or the caller
		// closing conn after Run returns).
		setErr(fmt.Errorf("client: %w", ctx.Err()))
		cancel()
		closeSend()
		<-allDone
	}

	resultMu.Lock()
	defer resultMu.Unlock()
	if runErr != nil {
		return Result{}, runErr
	}
	return result, nil
}

// dispatch acts on one server-originated frame. Returns done=true
// when the caller should stop reading (Exit received). Any protocol
// violation or write error propagates as the returned error.
func dispatch(f proto.Frame, t Terminal, setResult func(Result)) (bool, error) {
	switch m := f.(type) {
	case *proto.Output:
		if len(m.Data) == 0 {
			return false, nil
		}
		if _, err := writeAll(t, m.Data); err != nil {
			return false, clientErr(OpDispatch, ErrLostConnection, err, "write output to terminal")
		}
		return false, nil
	case *proto.Exit:
		setResult(Result{ExitReason: m.Reason, ExitMessage: m.Message})
		return true, nil
	case *proto.CommandResult:
		if m.Status != proto.StatusOk {
			log.Printf("client: command %d returned %s: %s", m.ID, m.Status, m.Message)
		}
		return false, nil
	case *proto.Beep:
		// M1: no-op. Real client will ring the local bell.
		return false, nil
	default:
		return false, clientErr(OpDispatch, ErrProtocol, nil, "unexpected frame %s", f.Type())
	}
}

// writeAll loops around t.Write in case the terminal returns short
// writes without an error (same portability concern xio.Writer
// addresses for io.Writer).
func writeAll(t Terminal, p []byte) (int, error) {
	total := 0
	for total < len(p) {
		n, err := t.Write(p[total:])
		if n > 0 {
			total += n
		}
		if err != nil {
			return total, err
		}
		if n == 0 {
			return total, io.ErrShortWrite
		}
	}
	return total, nil
}

// handshake runs the pre-goroutine exchange: Identify, then
// CommandList (if any), then draining the matching CommandResults.
// Returns a wrapped ErrHandshake on failure.
func handshake(conn net.Conn, t Terminal, opts Options) error {
	cols, rows, err := t.Size()
	if err != nil {
		return clientErr(OpHandshake, ErrHandshake, err, "probe terminal size")
	}

	fw := xio.NewWriter(conn)
	fr := xio.NewReader(conn)

	// TODO(m1:client-caps): probe DA2 and KKP, fill Features.
	// For now we send opts.Profile verbatim and no Features.
	ident := &proto.Identify{
		ProtocolVersion: proto.ProtocolVersion,
		Profile:         opts.Profile,
		InitialCols:     uint32(cols),
		InitialRows:     uint32(rows),
		Cwd:             opts.Cwd,
		TTYName:         opts.TTYName,
		TermEnv:         opts.TermEnv,
		Env:             opts.Env,
	}
	if err := fw.WriteFrame(ident); err != nil {
		return clientErr(OpHandshake, ErrHandshake, err, "write Identify")
	}

	if len(opts.Commands) == 0 {
		return nil
	}

	cl := &proto.CommandList{Commands: opts.Commands}
	if err := fw.WriteFrame(cl); err != nil {
		return clientErr(OpHandshake, ErrHandshake, err, "write CommandList")
	}

	for i := 0; i < len(opts.Commands); i++ {
		f, err := fr.ReadFrame()
		if err != nil {
			return clientErr(OpHandshake, ErrHandshake, err, "read CommandResult")
		}
		res, ok := f.(*proto.CommandResult)
		if !ok {
			return clientErr(OpHandshake, ErrProtocol, nil,
				"expected CommandResult, got %s", f.Type())
		}
		if res.Status != proto.StatusOk {
			// M2 will route these back to the caller. For now we
			// log and continue per CLAUDE.md rule 6 (no double
			// log-and-return): the attach still proceeds.
			log.Printf("client: command %d returned %s: %s", res.ID, res.Status, res.Message)
		}
	}
	return nil
}

// isCanceled reports whether ctx has been canceled without blocking.
func isCanceled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// TODO(m1:client-bye): when M2-1 implements Ctrl-B-d, a clean-detach
// path sends proto.Bye via the send channel and waits for the
// server's Exit reply. The writer goroutine is already serialized,
// so enqueuing &proto.Bye{} at that point is the only change.
