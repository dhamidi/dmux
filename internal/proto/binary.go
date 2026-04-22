package proto

import (
	"encoding/binary"
	"io"
)

// bwriter accumulates little-endian encoded fields in a byte slice
// with a latched error: the first failure sticks, subsequent writes
// are no-ops. This keeps MarshalBinary bodies flat and linear.
//
// op and typ are set by the caller so that errors produced inside
// the helper carry the correct Op/Type context without the caller
// having to re-wrap them.
type bwriter struct {
	op  Op
	typ MsgType
	buf []byte
	err error
}

func (w *bwriter) bytes() []byte { return w.buf }

func (w *bwriter) u8(v uint8) {
	if w.err != nil {
		return
	}
	w.buf = append(w.buf, v)
}

func (w *bwriter) u32(v uint32) {
	if w.err != nil {
		return
	}
	w.buf = binary.LittleEndian.AppendUint32(w.buf, v)
}

func (w *bwriter) string(s string) {
	if w.err != nil {
		return
	}
	if uint64(len(s)) > MaxFrameSize {
		w.err = frameErr(w.op, w.typ, ErrPayloadTooLarge, "string length %d", len(s))
		return
	}
	w.u32(uint32(len(s)))
	w.buf = append(w.buf, s...)
}

func (w *bwriter) stringSlice(ss []string) {
	if w.err != nil {
		return
	}
	if uint64(len(ss)) > MaxFrameSize {
		w.err = frameErr(w.op, w.typ, ErrPayloadTooLarge, "slice count %d", len(ss))
		return
	}
	w.u32(uint32(len(ss)))
	for _, s := range ss {
		w.string(s)
	}
}

// breader consumes little-endian fields from a byte slice with a
// latched error. Call finish at the end of UnmarshalBinary to
// surface errors and check for trailing bytes.
type breader struct {
	op  Op
	typ MsgType
	buf []byte
	err error
}

func (r *breader) u8() uint8 {
	if r.err != nil {
		return 0
	}
	if len(r.buf) < 1 {
		r.err = frameErr(r.op, r.typ, io.ErrUnexpectedEOF, "")
		return 0
	}
	v := r.buf[0]
	r.buf = r.buf[1:]
	return v
}

func (r *breader) u32() uint32 {
	if r.err != nil {
		return 0
	}
	if len(r.buf) < 4 {
		r.err = frameErr(r.op, r.typ, io.ErrUnexpectedEOF, "")
		return 0
	}
	v := binary.LittleEndian.Uint32(r.buf[:4])
	r.buf = r.buf[4:]
	return v
}

func (r *breader) string() string {
	n := r.u32()
	if r.err != nil {
		return ""
	}
	if uint64(n) > MaxFrameSize {
		r.err = frameErr(r.op, r.typ, ErrPayloadTooLarge, "string length %d", n)
		return ""
	}
	if int(n) > len(r.buf) {
		r.err = frameErr(r.op, r.typ, io.ErrUnexpectedEOF, "want %d bytes, have %d", n, len(r.buf))
		return ""
	}
	s := string(r.buf[:n])
	r.buf = r.buf[n:]
	return s
}

func (r *breader) stringSlice() []string {
	n := r.u32()
	if r.err != nil {
		return nil
	}
	if uint64(n) > MaxFrameSize {
		r.err = frameErr(r.op, r.typ, ErrPayloadTooLarge, "slice count %d", n)
		return nil
	}
	// Cap the up-front allocation against the bytes that actually
	// remain: each entry is at least a 4-byte lenstr header, so
	// len(r.buf)/4 is a hard ceiling on reachable entries. Without
	// this, a 4-byte payload claiming count=1M forces a ~16 MiB
	// header allocation before the loop discovers EOF.
	if int(n) > len(r.buf)/4 {
		r.err = frameErr(r.op, r.typ, ErrMalformed, "slice count %d exceeds remaining bytes", n)
		return nil
	}
	ss := make([]string, n)
	for i := range ss {
		ss[i] = r.string()
		if r.err != nil {
			return nil
		}
	}
	return ss
}

func (r *breader) finish() error {
	if r.err != nil {
		return r.err
	}
	if len(r.buf) > 0 {
		return frameErr(r.op, r.typ, ErrMalformed, "%d trailing bytes", len(r.buf))
	}
	return nil
}
