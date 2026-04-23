package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/tty"
	"github.com/dhamidi/dmux/internal/xio"
)

// fakeTerm is a scripted Terminal: reads come from a channel the
// test drives, writes land in a buffer the test inspects, and
// resize events come from a channel the test fires.
type fakeTerm struct {
	reads    chan []byte
	readErr  chan error
	writesMu sync.Mutex
	writes   bytes.Buffer
	cols     int
	rows     int
	resizeCh chan tty.ResizeEvent
}

func newFakeTerm() *fakeTerm {
	return &fakeTerm{
		reads:    make(chan []byte, 4),
		readErr:  make(chan error, 1),
		cols:     80,
		rows:     24,
		resizeCh: make(chan tty.ResizeEvent, 1),
	}
}

func (f *fakeTerm) Read(p []byte) (int, error) {
	select {
	case b, ok := <-f.reads:
		if !ok {
			return 0, io.EOF
		}
		return copy(p, b), nil
	case err := <-f.readErr:
		return 0, err
	}
}

func (f *fakeTerm) Write(p []byte) (int, error) {
	f.writesMu.Lock()
	defer f.writesMu.Unlock()
	return f.writes.Write(p)
}

func (f *fakeTerm) Size() (int, int, error) { return f.cols, f.rows, nil }

func (f *fakeTerm) Resize() <-chan tty.ResizeEvent { return f.resizeCh }

func (f *fakeTerm) writesBytes() []byte {
	f.writesMu.Lock()
	defer f.writesMu.Unlock()
	return append([]byte(nil), f.writes.Bytes()...)
}

// scriptedServer pretends to be the dmux server on the other end of
// a net.Pipe connection. It is used by tests to script handshake
// responses and record what the client sent.
type scriptedServer struct {
	conn   net.Conn
	r      xio.FrameReader
	w      xio.FrameWriter
	framesMu sync.Mutex
	frames []proto.Frame
}

func newScriptedServer(conn net.Conn) *scriptedServer {
	return &scriptedServer{
		conn: conn,
		r:    xio.NewReader(conn),
		w:    xio.NewWriter(conn),
	}
}

// expect reads one frame and records it. On error returns the error
// so the test can Fatal from the goroutine via a channel.
func (s *scriptedServer) expect() (proto.Frame, error) {
	f, err := s.r.ReadFrame()
	if err != nil {
		return nil, err
	}
	s.framesMu.Lock()
	s.frames = append(s.frames, f)
	s.framesMu.Unlock()
	return f, nil
}

func (s *scriptedServer) received() []proto.Frame {
	s.framesMu.Lock()
	defer s.framesMu.Unlock()
	out := make([]proto.Frame, len(s.frames))
	copy(out, s.frames)
	return out
}

// runWithTimeout guards Run calls so a hanging test fails fast.
func runWithTimeout(t *testing.T, fn func() (Result, error)) (Result, error) {
	t.Helper()
	type ret struct {
		r   Result
		err error
	}
	ch := make(chan ret, 1)
	go func() {
		r, err := fn()
		ch <- ret{r, err}
	}()
	select {
	case v := <-ch:
		return v.r, v.err
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return within 3s")
		return Result{}, nil
	}
}

func sampleCommand(id uint32, argv ...string) proto.Command {
	return proto.Command{ID: id, Argv: argv}
}

// TestHandshakeAndExit covers the happy path: Identify + CommandList,
// server replies with one CommandResult{Ok} and then Exit{Detached}.
func TestHandshakeAndExit(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	srv := newScriptedServer(serverConn)
	srvDone := make(chan error, 1)
	go func() {
		// Expect Identify.
		f, err := srv.expect()
		if err != nil {
			srvDone <- err
			return
		}
		if f.Type() != proto.MsgIdentify {
			srvDone <- errors.New("expected Identify")
			return
		}
		// Expect CommandList.
		f, err = srv.expect()
		if err != nil {
			srvDone <- err
			return
		}
		cl, ok := f.(*proto.CommandList)
		if !ok || len(cl.Commands) != 1 {
			srvDone <- errors.New("expected CommandList with one command")
			return
		}
		// Reply CommandResult{Ok} + Exit.
		if err := srv.w.WriteFrame(&proto.CommandResult{ID: cl.Commands[0].ID, Status: proto.StatusOk}); err != nil {
			srvDone <- err
			return
		}
		if err := srv.w.WriteFrame(&proto.Exit{Reason: proto.ExitDetached, Message: "bye"}); err != nil {
			srvDone <- err
			return
		}
		srvDone <- nil
	}()

	term := newFakeTerm()
	opts := Options{Commands: []proto.Command{sampleCommand(1, "attach-session")}}
	res, err := runWithTimeout(t, func() (Result, error) {
		return Run(context.Background(), clientConn, term, opts)
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitReason != proto.ExitDetached {
		t.Errorf("ExitReason = %v, want %v", res.ExitReason, proto.ExitDetached)
	}
	if res.ExitMessage != "bye" {
		t.Errorf("ExitMessage = %q, want %q", res.ExitMessage, "bye")
	}
	if err := <-srvDone; err != nil {
		t.Fatalf("server goroutine: %v", err)
	}
}

// TestInputPassthrough: fake terminal yields one Read; server
// observes an Input frame with the exact bytes.
func TestInputPassthrough(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	srv := newScriptedServer(serverConn)
	gotInput := make(chan []byte, 1)
	srvDone := make(chan error, 1)
	go func() {
		if _, err := srv.expect(); err != nil { // Identify
			srvDone <- err
			return
		}
		// No CommandList in this scenario (opts.Commands empty),
		// so the next frame is whatever the client sends first —
		// the stdin goroutine will produce an Input frame.
		for {
			f, err := srv.expect()
			if err != nil {
				srvDone <- err
				return
			}
			if in, ok := f.(*proto.Input); ok {
				gotInput <- append([]byte(nil), in.Data...)
				// Signal session end.
				if err := srv.w.WriteFrame(&proto.Exit{Reason: proto.ExitDetached}); err != nil {
					srvDone <- err
					return
				}
				srvDone <- nil
				return
			}
		}
	}()

	term := newFakeTerm()
	term.reads <- []byte("hi\n")

	res, err := runWithTimeout(t, func() (Result, error) {
		return Run(context.Background(), clientConn, term, Options{})
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitReason != proto.ExitDetached {
		t.Errorf("ExitReason = %v", res.ExitReason)
	}

	select {
	case b := <-gotInput:
		if string(b) != "hi\n" {
			t.Errorf("Input payload = %q, want %q", b, "hi\n")
		}
	default:
		t.Fatal("server did not observe Input frame")
	}
	if err := <-srvDone; err != nil {
		t.Fatalf("server goroutine: %v", err)
	}
}

// TestOutputPassthrough: server sends Output then Exit; fake
// terminal's Write should receive the output before Run returns.
func TestOutputPassthrough(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	srv := newScriptedServer(serverConn)
	srvDone := make(chan error, 1)
	go func() {
		if _, err := srv.expect(); err != nil {
			srvDone <- err
			return
		}
		if err := srv.w.WriteFrame(&proto.Output{Data: []byte("world")}); err != nil {
			srvDone <- err
			return
		}
		if err := srv.w.WriteFrame(&proto.Exit{Reason: proto.ExitServerExit}); err != nil {
			srvDone <- err
			return
		}
		srvDone <- nil
	}()

	term := newFakeTerm()
	res, err := runWithTimeout(t, func() (Result, error) {
		return Run(context.Background(), clientConn, term, Options{})
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitReason != proto.ExitServerExit {
		t.Errorf("ExitReason = %v, want %v", res.ExitReason, proto.ExitServerExit)
	}
	if got := string(term.writesBytes()); got != "world" {
		t.Errorf("terminal writes = %q, want %q", got, "world")
	}
	if err := <-srvDone; err != nil {
		t.Fatalf("server goroutine: %v", err)
	}
}

// TestResizeFrame: a ResizeEvent fires on the fake terminal's
// channel; server observes a Resize frame with matching dims.
func TestResizeFrame(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	srv := newScriptedServer(serverConn)
	gotResize := make(chan *proto.Resize, 1)
	srvDone := make(chan error, 1)
	go func() {
		if _, err := srv.expect(); err != nil {
			srvDone <- err
			return
		}
		for {
			f, err := srv.expect()
			if err != nil {
				srvDone <- err
				return
			}
			if rz, ok := f.(*proto.Resize); ok {
				gotResize <- rz
				if err := srv.w.WriteFrame(&proto.Exit{Reason: proto.ExitDetached}); err != nil {
					srvDone <- err
					return
				}
				srvDone <- nil
				return
			}
		}
	}()

	term := newFakeTerm()
	term.resizeCh <- tty.ResizeEvent{Cols: 120, Rows: 40}

	_, err := runWithTimeout(t, func() (Result, error) {
		return Run(context.Background(), clientConn, term, Options{})
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case rz := <-gotResize:
		if rz.Cols != 120 || rz.Rows != 40 {
			t.Errorf("Resize = %dx%d, want 120x40", rz.Cols, rz.Rows)
		}
	default:
		t.Fatal("server did not observe Resize")
	}
	if err := <-srvDone; err != nil {
		t.Fatalf("server goroutine: %v", err)
	}
}

// TestLostConnection: server closes its end mid-session; Run
// returns an error wrapping ErrLostConnection.
func TestLostConnection(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	srv := newScriptedServer(serverConn)
	srvDone := make(chan error, 1)
	go func() {
		if _, err := srv.expect(); err != nil {
			srvDone <- err
			return
		}
		// Close the connection: client's reader should see EOF.
		serverConn.Close()
		srvDone <- nil
	}()

	term := newFakeTerm()
	_, err := runWithTimeout(t, func() (Result, error) {
		return Run(context.Background(), clientConn, term, Options{})
	})
	if err == nil {
		t.Fatal("Run returned nil, want error")
	}
	if !errors.Is(err, ErrLostConnection) {
		t.Errorf("err = %v, want to wrap ErrLostConnection", err)
	}
	var ce *ClientError
	if !errors.As(err, &ce) {
		t.Errorf("err = %v, want *ClientError", err)
	}
	if err := <-srvDone; err != nil {
		t.Fatalf("server goroutine: %v", err)
	}
}

// TestContextCancel: cancel the context; Run returns ctx.Err().
func TestContextCancel(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	srv := newScriptedServer(serverConn)
	srvDone := make(chan error, 1)
	go func() {
		if _, err := srv.expect(); err != nil { // Identify
			srvDone <- err
			return
		}
		// Park: read until the client's conn closes. We keep the
		// server side idle so only ctx cancel can unblock Run.
		for {
			if _, err := srv.expect(); err != nil {
				srvDone <- nil
				return
			}
		}
	}()

	term := newFakeTerm()
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay so the client reaches steady state.
	time.AfterFunc(100*time.Millisecond, func() {
		cancel()
		// The reader is parked in ReadFrame; closing the client
		// conn unblocks it so Run can actually return. This
		// matches the documented contract: the caller owns conn.
		clientConn.Close()
	})

	_, err := runWithTimeout(t, func() (Result, error) {
		return Run(ctx, clientConn, term, Options{})
	})
	if err == nil {
		t.Fatal("Run returned nil, want context error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want to wrap context.Canceled", err)
	}
	<-srvDone
}
