package proto

import (
	"encoding"
	"fmt"
)

// MsgType identifies a frame's concrete payload type. Values are
// stable across releases; new types take new values.
type MsgType uint16

// Client -> server message types.
const (
	MsgIdentify    MsgType = 0x0001
	MsgCommandList MsgType = 0x0002
	MsgInput       MsgType = 0x0003
	MsgResize      MsgType = 0x0004
	MsgCapsUpdate  MsgType = 0x0005
	MsgBye         MsgType = 0x0006
)

// Server -> client message types. The 0x0080 bit is set on every
// server-origin type; see MsgType.IsServerToClient.
const (
	MsgOutput        MsgType = 0x0080
	MsgCommandResult MsgType = 0x0081
	MsgExit          MsgType = 0x0082
	MsgBeep          MsgType = 0x0083
)

// serverBit flags a server-originated message type.
const serverBit = 0x0080

// IsServerToClient reports whether the message type is sent by the server.
func (t MsgType) IsServerToClient() bool { return t&serverBit != 0 }

// String returns a short name for logs and errors.
func (t MsgType) String() string {
	switch t {
	case MsgIdentify:
		return "Identify"
	case MsgCommandList:
		return "CommandList"
	case MsgInput:
		return "Input"
	case MsgResize:
		return "Resize"
	case MsgCapsUpdate:
		return "CapsUpdate"
	case MsgBye:
		return "Bye"
	case MsgOutput:
		return "Output"
	case MsgCommandResult:
		return "CommandResult"
	case MsgExit:
		return "Exit"
	case MsgBeep:
		return "Beep"
	default:
		return fmt.Sprintf("MsgType(%#04x)", uint16(t))
	}
}

// EnvelopeSize is the byte count of the fixed per-frame header,
// including the length field itself.
//
//	length u32 | type u16 | flags u16 | reserved u32 = 12 bytes
const EnvelopeSize = 12

// HeaderSize is the portion of the envelope covered by the length
// field: type + flags + reserved. The length field value equals
// HeaderSize + payload length.
const HeaderSize = 8

// MaxFrameSize caps a frame's length-field value (and therefore the
// payload). Receivers reject frames whose length exceeds this.
const MaxFrameSize = 1 << 20 // 1 MiB

// ProtocolVersion is the current value sent in Identify.
const ProtocolVersion uint8 = 1

// Frame is a protocol message. Implementations are pointer-receiver
// structs exported from this package (*Identify, *Input, ...).
type Frame interface {
	Type() MsgType
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

// NewFrame returns a zero-valued Frame for the given type, ready to
// have UnmarshalBinary called on it. Returns ErrUnknownType when
// the type is not recognized.
func NewFrame(t MsgType) (Frame, error) {
	switch t {
	case MsgIdentify:
		return &Identify{}, nil
	case MsgCommandList:
		return &CommandList{}, nil
	case MsgInput:
		return &Input{}, nil
	case MsgResize:
		return &Resize{}, nil
	case MsgCapsUpdate:
		return &CapsUpdate{}, nil
	case MsgBye:
		return &Bye{}, nil
	case MsgOutput:
		return &Output{}, nil
	case MsgCommandResult:
		return &CommandResult{}, nil
	case MsgExit:
		return &Exit{}, nil
	case MsgBeep:
		return &Beep{}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownType, t)
	}
}
