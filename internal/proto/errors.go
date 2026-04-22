package proto

import "errors"

// Sentinel errors. Callers compare with errors.Is.
var (
	// ErrUnknownType is returned by NewFrame when asked for a MsgType
	// this package does not recognize.
	ErrUnknownType = errors.New("proto: unknown frame type")

	// ErrShortBuffer is returned when a caller-provided buffer is
	// smaller than the operation requires.
	ErrShortBuffer = errors.New("proto: short buffer")

	// ErrMalformed is returned when bytes cannot be parsed as a
	// frame of the declared type (invalid enum value, trailing
	// bytes, out-of-range count, ...).
	ErrMalformed = errors.New("proto: malformed frame")

	// ErrPayloadTooLarge is returned when a frame or variable-width
	// field exceeds MaxFrameSize.
	ErrPayloadTooLarge = errors.New("proto: payload too large")
)
