package proto

import (
	"encoding/binary"
	"fmt"
)

// EncodeEnvelope writes the 12-byte frame envelope into dst[0:12].
// payloadLen is the byte count of the payload that will follow; the
// function rejects payloads that would push the frame above
// MaxFrameSize. Returns ErrShortBuffer if dst is smaller than
// EnvelopeSize.
//
// The envelope layout is:
//
//	length u32 | type u16 | flags u16 | reserved u32
//
// where length is HeaderSize + payloadLen (little-endian, as every
// other multi-byte integer in this package).
func EncodeEnvelope(dst []byte, t MsgType, flags uint16, payloadLen uint32) error {
	if len(dst) < EnvelopeSize {
		return ErrShortBuffer
	}
	if payloadLen > MaxFrameSize-HeaderSize {
		return fmt.Errorf("%w: payload %d bytes", ErrPayloadTooLarge, payloadLen)
	}
	binary.LittleEndian.PutUint32(dst[0:4], HeaderSize+payloadLen)
	binary.LittleEndian.PutUint16(dst[4:6], uint16(t))
	binary.LittleEndian.PutUint16(dst[6:8], flags)
	binary.LittleEndian.PutUint32(dst[8:12], 0)
	return nil
}

// DecodeEnvelope parses a 12-byte frame envelope from src[0:12].
// payloadLen is the number of bytes that should follow to complete
// the frame. Returns ErrShortBuffer if src is smaller than
// EnvelopeSize, ErrMalformed if the length field is below
// HeaderSize, or ErrPayloadTooLarge if it exceeds MaxFrameSize.
func DecodeEnvelope(src []byte) (t MsgType, flags uint16, payloadLen uint32, err error) {
	if len(src) < EnvelopeSize {
		return 0, 0, 0, ErrShortBuffer
	}
	length := binary.LittleEndian.Uint32(src[0:4])
	t = MsgType(binary.LittleEndian.Uint16(src[4:6]))
	flags = binary.LittleEndian.Uint16(src[6:8])
	// src[8:12] is the reserved sequence-number slot; unused in M1.
	if length < HeaderSize {
		return 0, 0, 0, fmt.Errorf("%w: length %d < header %d", ErrMalformed, length, HeaderSize)
	}
	if length > MaxFrameSize {
		return 0, 0, 0, fmt.Errorf("%w: length %d > max %d", ErrPayloadTooLarge, length, MaxFrameSize)
	}
	return t, flags, length - HeaderSize, nil
}
