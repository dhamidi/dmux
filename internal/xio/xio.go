package xio

import (
	"bufio"
	"fmt"
	"io"

	"github.com/dhamidi/dmux/internal/proto"
)

// FrameReader reads proto.Frame values from an underlying byte stream.
// Instances are owned by exactly one goroutine.
type FrameReader interface {
	ReadFrame() (proto.Frame, error)
}

// FrameWriter writes proto.Frame values to an underlying byte stream.
// Callers must serialize WriteFrame calls on a single writer; there is
// no internal locking.
type FrameWriter interface {
	WriteFrame(proto.Frame) error
}

// NewReader wraps r so each ReadFrame returns one frame. If r is not
// already a *bufio.Reader, it is wrapped in one to coalesce small
// envelope + payload reads into a single syscall.
func NewReader(r io.Reader) FrameReader {
	if _, ok := r.(*bufio.Reader); !ok {
		r = bufio.NewReader(r)
	}
	return &reader{r: r}
}

// NewWriter returns a FrameWriter that writes each frame to w in a
// single w.Write call (envelope + payload). Writes are synchronous;
// backpressure from w propagates to the caller by blocking.
func NewWriter(w io.Writer) FrameWriter {
	return &writer{w: w}
}

type reader struct {
	r   io.Reader
	hdr [proto.EnvelopeSize]byte
	buf []byte // reused across frames
}

// ReadFrame reads one frame. Returns io.EOF if the stream is cleanly
// closed at a frame boundary, io.ErrUnexpectedEOF if it is closed
// mid-frame, or a wrapped proto error on malformed input.
func (fr *reader) ReadFrame() (proto.Frame, error) {
	if _, err := io.ReadFull(fr.r, fr.hdr[:]); err != nil {
		return nil, err
	}
	typ, _, payloadLen, err := proto.DecodeEnvelope(fr.hdr[:])
	if err != nil {
		return nil, fmt.Errorf("xio: decode envelope: %w", err)
	}
	if cap(fr.buf) < int(payloadLen) {
		fr.buf = make([]byte, payloadLen)
	} else {
		fr.buf = fr.buf[:payloadLen]
	}
	if _, err := io.ReadFull(fr.r, fr.buf); err != nil {
		return nil, fmt.Errorf("xio: read payload: %w", err)
	}
	f, err := proto.NewFrame(typ)
	if err != nil {
		return nil, fmt.Errorf("xio: %w", err)
	}
	if err := f.UnmarshalBinary(fr.buf); err != nil {
		return nil, fmt.Errorf("xio: unmarshal %s: %w", typ, err)
	}
	return f, nil
}

type writer struct {
	w   io.Writer
	buf []byte // reused across frames
}

// WriteFrame marshals f, then writes envelope + payload in one Write.
func (fw *writer) WriteFrame(f proto.Frame) error {
	payload, err := f.MarshalBinary()
	if err != nil {
		return fmt.Errorf("xio: marshal %s: %w", f.Type(), err)
	}
	// Pre-check before allocating the combined buffer; EncodeEnvelope
	// rejects the same condition, but rejecting earlier avoids a
	// potentially-huge make() on a malformed Frame.
	if uint64(len(payload)) > proto.MaxFrameSize-proto.HeaderSize {
		return fmt.Errorf("xio: %w: %s payload %d bytes", proto.ErrPayloadTooLarge, f.Type(), len(payload))
	}
	total := proto.EnvelopeSize + len(payload)
	if cap(fw.buf) < total {
		fw.buf = make([]byte, total)
	} else {
		fw.buf = fw.buf[:total]
	}
	// flags is always 0 in M1; see proto/doc.go for the reservation.
	if err := proto.EncodeEnvelope(fw.buf[:proto.EnvelopeSize], f.Type(), 0, uint32(len(payload))); err != nil {
		return fmt.Errorf("xio: encode envelope: %w", err)
	}
	copy(fw.buf[proto.EnvelopeSize:], payload)
	// Loop around Write: the io.Writer contract permits n < len(p)
	// paired with a non-nil error, but some callers still produce
	// short writes with err == nil. Looping is the portable fix.
	for off := 0; off < len(fw.buf); {
		n, err := fw.w.Write(fw.buf[off:])
		if err != nil {
			return fmt.Errorf("xio: write: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("xio: write: %w", io.ErrShortWrite)
		}
		off += n
	}
	return nil
}
