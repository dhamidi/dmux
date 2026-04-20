package proto

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	// headerSize is the size of the fixed message header in bytes:
	// 4 bytes MsgType (big-endian uint32) + 4 bytes payload length (big-endian uint32).
	headerSize = 8

	// maxPayload is the maximum allowed payload size (16 MiB).
	maxPayload = 16 << 20
)

// ReadMsg reads one framed message from r and returns its type and raw
// payload bytes. r must be an io.Reader; no network type is required.
// The returned payload is only valid until the next call to ReadMsg.
func ReadMsg(r io.Reader) (MsgType, []byte, error) {
	var hdr [headerSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	t := MsgType(binary.BigEndian.Uint32(hdr[0:4]))
	n := binary.BigEndian.Uint32(hdr[4:8])
	if n > maxPayload {
		return 0, nil, fmt.Errorf("proto: payload too large: %d bytes", n)
	}
	payload := make([]byte, n)
	if n > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return 0, nil, fmt.Errorf("proto: reading payload for %s: %w", t, err)
		}
	}
	return t, payload, nil
}

// WriteMsg writes one framed message to w. w must be an io.Writer; no
// network type is required. payload may be nil or empty for messages with
// no body.
func WriteMsg(w io.Writer, t MsgType, payload []byte) error {
	var hdr [headerSize]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(t))
	binary.BigEndian.PutUint32(hdr[4:8], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}
