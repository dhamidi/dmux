package proto

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors. Callers use errors.Is to dispatch on category.
// The concrete error returned by this package is usually a
// *FrameError wrapping one of these, so errors.As can also pull
// out operation and frame-type context.
var (
	// ErrUnknownType is returned by NewFrame and by envelope
	// decoding when the MsgType is not recognized.
	ErrUnknownType = errors.New("proto: unknown frame type")

	// ErrShortBuffer is returned when a caller-provided buffer is
	// smaller than the operation requires (e.g. envelope encode).
	ErrShortBuffer = errors.New("proto: short buffer")

	// ErrMalformed is returned when bytes cannot be parsed as a
	// frame of the declared type: invalid enum value, trailing
	// bytes, out-of-range count, truncated payload.
	ErrMalformed = errors.New("proto: malformed frame")

	// ErrPayloadTooLarge is returned when a frame or a variable-
	// width field inside a frame exceeds MaxFrameSize.
	ErrPayloadTooLarge = errors.New("proto: payload too large")
)

// Op describes what the package was doing when an error arose.
// It is part of FrameError's public shape so callers can log or
// dispatch on the operation without parsing Error().
type Op string

const (
	OpMarshal        Op = "marshal"
	OpUnmarshal      Op = "unmarshal"
	OpEncodeEnvelope Op = "encode-envelope"
	OpDecodeEnvelope Op = "decode-envelope"
	OpNewFrame       Op = "new-frame"
)

// FrameError is the concrete error type returned by this package.
// It carries an Op, the MsgType involved (zero if unknown or not
// yet parsed), a free-form Detail for humans, and wraps one of the
// sentinel errors so errors.Is works.
//
// Callers:
//
//	var fe *proto.FrameError
//	if errors.As(err, &fe) {
//	    // fe.Op, fe.Type, fe.Detail are available
//	}
//	if errors.Is(err, proto.ErrMalformed) { ... }
type FrameError struct {
	Op     Op
	Type   MsgType // 0 when not applicable or not yet parsed
	Detail string  // free-form context for logs; may be empty
	Err    error   // one of the sentinels above
}

func (e *FrameError) Error() string {
	var b strings.Builder
	b.WriteString("proto: ")
	b.WriteString(string(e.Op))
	if e.Type != 0 {
		b.WriteByte(' ')
		b.WriteString(e.Type.String())
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

func (e *FrameError) Unwrap() error { return e.Err }

// frameErr builds a *FrameError for internal call sites. The
// detail string is formatted once to keep allocation profile flat.
func frameErr(op Op, t MsgType, cause error, format string, args ...any) error {
	var detail string
	if format != "" {
		detail = fmt.Sprintf(format, args...)
	}
	return &FrameError{Op: op, Type: t, Detail: detail, Err: cause}
}
