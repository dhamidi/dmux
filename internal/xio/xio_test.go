package xio_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"reflect"
	"sync"
	"testing"

	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/xio"
)

// TestRoundtripOverPipe is the integration checkpoint: every M1
// frame type is sent by one goroutine through a net.Pipe and
// received by another. A failure here would be the first sign
// proto and xio disagree on the wire format.
func TestRoundtripOverPipe(t *testing.T) {
	frames := []proto.Frame{
		&proto.Identify{
			ProtocolVersion: proto.ProtocolVersion,
			Profile:         0,
			InitialCols:     80, InitialRows: 24,
			Cwd: "/tmp", TTYName: "/dev/pts/0", TermEnv: "xterm-256color",
			Env: []string{"HOME=/tmp"}, Features: []uint8{},
		},
		&proto.CommandList{Commands: []proto.Command{
			{ID: 1, Argv: []string{"new-session"}},
			{ID: 2, Argv: []string{"attach-session"}},
		}},
		&proto.Input{Data: []byte("ls\n")},
		&proto.Resize{Cols: 100, Rows: 30},
		&proto.CapsUpdate{Profile: 1},
		&proto.Bye{},
		&proto.Output{Data: []byte("hello world")},
		&proto.CommandResult{ID: 1, Status: proto.StatusOk},
		&proto.CommandResult{ID: 2, Status: proto.StatusError, Message: "nope"},
		&proto.Exit{Reason: proto.ExitDetached, Message: "bye"},
		&proto.Beep{},
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		w := xio.NewWriter(clientConn)
		for _, f := range frames {
			if err := w.WriteFrame(f); err != nil {
				t.Errorf("WriteFrame %s: %v", f.Type(), err)
				return
			}
		}
		clientConn.Close()
	}()

	r := xio.NewReader(serverConn)
	for i, want := range frames {
		got, err := r.ReadFrame()
		if err != nil {
			t.Fatalf("ReadFrame[%d] (%s): %v", i, want.Type(), err)
		}
		if got.Type() != want.Type() {
			t.Errorf("ReadFrame[%d] type = %s, want %s", i, got.Type(), want.Type())
		}
		if !wireEqual(got, want) {
			t.Errorf("ReadFrame[%d] payload mismatch:\n got=%#v\nwant=%#v", i, got, want)
		}
	}
	if _, err := r.ReadFrame(); !errors.Is(err, io.EOF) {
		t.Errorf("trailing ReadFrame: got %v, want io.EOF", err)
	}
	wg.Wait()
}

func wireEqual(a, b proto.Frame) bool {
	ab, _ := a.MarshalBinary()
	bb, _ := b.MarshalBinary()
	return bytes.Equal(ab, bb)
}

func TestReadFrameCleanEOF(t *testing.T) {
	// Empty stream should return io.EOF at the first read.
	r := xio.NewReader(bytes.NewReader(nil))
	_, err := r.ReadFrame()
	if !errors.Is(err, io.EOF) {
		t.Errorf("got %v, want io.EOF", err)
	}
}

func TestReadFrameTruncatedHeader(t *testing.T) {
	// Five bytes, less than an envelope.
	r := xio.NewReader(bytes.NewReader([]byte{1, 2, 3, 4, 5}))
	_, err := r.ReadFrame()
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("got %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestReadFrameTruncatedPayload(t *testing.T) {
	// Valid envelope claiming 10 payload bytes, followed by 3.
	var buf bytes.Buffer
	hdr := make([]byte, proto.EnvelopeSize)
	if err := proto.EncodeEnvelope(hdr, proto.MsgInput, 0, 10); err != nil {
		t.Fatal(err)
	}
	buf.Write(hdr)
	buf.Write([]byte{1, 2, 3})

	r := xio.NewReader(&buf)
	_, err := r.ReadFrame()
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("got %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestReadFrameUnknownType(t *testing.T) {
	var hdr [proto.EnvelopeSize]byte
	binary.LittleEndian.PutUint32(hdr[0:4], proto.HeaderSize) // length: no payload
	binary.LittleEndian.PutUint16(hdr[4:6], 0xABCD)           // bogus type
	r := xio.NewReader(bytes.NewReader(hdr[:]))
	_, err := r.ReadFrame()
	if !errors.Is(err, proto.ErrUnknownType) {
		t.Errorf("got %v, want ErrUnknownType", err)
	}
}

func TestReadFrameOversizeRejected(t *testing.T) {
	var hdr [proto.EnvelopeSize]byte
	binary.LittleEndian.PutUint32(hdr[0:4], proto.MaxFrameSize+1)
	binary.LittleEndian.PutUint16(hdr[4:6], uint16(proto.MsgInput))
	r := xio.NewReader(bytes.NewReader(hdr[:]))
	_, err := r.ReadFrame()
	if !errors.Is(err, proto.ErrPayloadTooLarge) {
		t.Errorf("got %v, want ErrPayloadTooLarge", err)
	}
}

func TestWriteFrameSingleWrite(t *testing.T) {
	// Verifies envelope + payload go out as one Write call,
	// which keeps frame boundaries intact on datagram-like
	// writers (not strictly required by io.Writer, but useful
	// for minimizing syscalls on net.Conn).
	var cw countingWriter
	w := xio.NewWriter(&cw)
	if err := w.WriteFrame(&proto.Input{Data: []byte("x")}); err != nil {
		t.Fatal(err)
	}
	if cw.calls != 1 {
		t.Errorf("Write calls = %d, want 1", cw.calls)
	}
	if cw.total != proto.EnvelopeSize+1 {
		t.Errorf("bytes written = %d, want %d", cw.total, proto.EnvelopeSize+1)
	}
}

type countingWriter struct {
	calls int
	total int
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.calls++
	c.total += len(p)
	return len(p), nil
}

func TestReadTwoFramesInOneStream(t *testing.T) {
	// Two back-to-back frames in the same stream must deserialize
	// independently. This also exercises bufio.Reader's coalescing
	// and the reader's payload-buffer reuse path.
	var buf bytes.Buffer
	w := xio.NewWriter(&buf)
	for i := 0; i < 2; i++ {
		if err := w.WriteFrame(&proto.Input{Data: bytes.Repeat([]byte{'a'}, 64)}); err != nil {
			t.Fatal(err)
		}
	}
	r := xio.NewReader(&buf)
	f1, err := r.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	f2, err := r.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f1, f2) {
		t.Errorf("frame1 != frame2: %#v vs %#v", f1, f2)
	}
}

// shortWriter returns len(p)/2 + 1 per call, producing short writes
// that fully drain across multiple calls. Used to verify WriteFrame
// loops correctly.
type shortWriter struct {
	dst   bytes.Buffer
	calls int
}

func (s *shortWriter) Write(p []byte) (int, error) {
	s.calls++
	n := len(p)/2 + 1
	if n > len(p) {
		n = len(p)
	}
	return s.dst.Write(p[:n])
}

func TestWriteFrameLoopsOnShortWrite(t *testing.T) {
	var sw shortWriter
	w := xio.NewWriter(&sw)
	payload := bytes.Repeat([]byte{'x'}, 100)
	if err := w.WriteFrame(&proto.Input{Data: payload}); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	if sw.calls < 2 {
		t.Errorf("short writes: got %d calls, want >= 2", sw.calls)
	}
	// Full frame should have made it through despite the short writes.
	r := xio.NewReader(&sw.dst)
	got, err := r.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	in, ok := got.(*proto.Input)
	if !ok {
		t.Fatalf("got %T, want *proto.Input", got)
	}
	if !bytes.Equal(in.Data, payload) {
		t.Errorf("payload mismatch after short writes")
	}
}

// stuckWriter always returns (0, nil). A correct WriteFrame must
// not spin forever on such a writer; it should detect no progress
// and return io.ErrShortWrite.
type stuckWriter struct{}

func (stuckWriter) Write(p []byte) (int, error) { return 0, nil }

func TestWriteFrameDetectsNoProgress(t *testing.T) {
	w := xio.NewWriter(stuckWriter{})
	err := w.WriteFrame(&proto.Input{Data: []byte("x")})
	if !errors.Is(err, io.ErrShortWrite) {
		t.Errorf("got %v, want io.ErrShortWrite", err)
	}
}
