package proto_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/dhamidi/dmux/internal/proto"
)

func TestFrameRoundtrip(t *testing.T) {
	cases := []proto.Frame{
		&proto.Identify{
			ProtocolVersion: proto.ProtocolVersion,
			Profile:         1,
			InitialCols:     120,
			InitialRows:     40,
			Cwd:             "/home/user",
			TTYName:         "/dev/pts/0",
			TermEnv:         "xterm-256color",
			Env:             []string{"HOME=/home/user", "PATH=/usr/bin", ""},
			Features:        []uint8{},
		},
		&proto.CommandList{Commands: []proto.Command{
			{ID: 1, Argv: []string{"new-session"}},
			{ID: 2, Argv: []string{"attach-session"}},
		}},
		&proto.Input{Data: []byte("hello\x1b[A")},
		&proto.Input{Data: []byte{}},
		&proto.Resize{Cols: 80, Rows: 24},
		&proto.CapsUpdate{Profile: 2},
		&proto.Bye{},
		&proto.Output{Data: []byte{0x1b, '[', 'H', 0x1b, '[', '2', 'J'}},
		&proto.CommandResult{ID: 1, Status: proto.StatusOk},
		&proto.CommandResult{ID: 2, Status: proto.StatusError, Message: "session exists"},
		&proto.CommandResult{ID: 3, Status: proto.StatusSkipped, Message: "previous failed"},
		&proto.Exit{Reason: proto.ExitKilled, Message: "kill-server"},
		&proto.Exit{Reason: proto.ExitDetached},
		&proto.Beep{},
	}

	for _, orig := range cases {
		t.Run(orig.Type().String(), func(t *testing.T) {
			payload, err := orig.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary: %v", err)
			}
			got, err := proto.NewFrame(orig.Type())
			if err != nil {
				t.Fatalf("NewFrame: %v", err)
			}
			if err := got.UnmarshalBinary(payload); err != nil {
				t.Fatalf("UnmarshalBinary: %v", err)
			}
			if !framesEqual(orig, got) {
				t.Fatalf("roundtrip mismatch\n orig=%#v\n  got=%#v", orig, got)
			}
		})
	}
}

// framesEqual tests wire-format equality: same type plus identical
// marshaled payloads. Distinct Go values (nil vs empty slice) that
// marshal to identical bytes are considered equal here.
func framesEqual(a, b proto.Frame) bool {
	if a.Type() != b.Type() {
		return false
	}
	ab, _ := a.MarshalBinary()
	bb, _ := b.MarshalBinary()
	return bytes.Equal(ab, bb)
}

func TestEnvelopeRoundtrip(t *testing.T) {
	var buf [proto.EnvelopeSize]byte
	if err := proto.EncodeEnvelope(buf[:], proto.MsgInput, 0, 10); err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}
	typ, flags, payloadLen, err := proto.DecodeEnvelope(buf[:])
	if err != nil {
		t.Fatalf("DecodeEnvelope: %v", err)
	}
	if typ != proto.MsgInput {
		t.Errorf("type=%v want %v", typ, proto.MsgInput)
	}
	if flags != 0 {
		t.Errorf("flags=%d want 0", flags)
	}
	if payloadLen != 10 {
		t.Errorf("payloadLen=%d want 10", payloadLen)
	}
}

func TestEnvelopeShortBuffer(t *testing.T) {
	if err := proto.EncodeEnvelope(make([]byte, 11), proto.MsgInput, 0, 0); !errors.Is(err, proto.ErrShortBuffer) {
		t.Errorf("EncodeEnvelope short: got %v, want ErrShortBuffer", err)
	}
	if _, _, _, err := proto.DecodeEnvelope(make([]byte, 11)); !errors.Is(err, proto.ErrShortBuffer) {
		t.Errorf("DecodeEnvelope short: got %v, want ErrShortBuffer", err)
	}
}

func TestEnvelopeTooLarge(t *testing.T) {
	var buf [proto.EnvelopeSize]byte
	err := proto.EncodeEnvelope(buf[:], proto.MsgInput, 0, proto.MaxFrameSize)
	if !errors.Is(err, proto.ErrPayloadTooLarge) {
		t.Errorf("EncodeEnvelope large: got %v, want ErrPayloadTooLarge", err)
	}

	// Forged envelope claiming an oversize length.
	binary.LittleEndian.PutUint32(buf[0:4], proto.MaxFrameSize+1)
	binary.LittleEndian.PutUint16(buf[4:6], uint16(proto.MsgInput))
	binary.LittleEndian.PutUint16(buf[6:8], 0)
	binary.LittleEndian.PutUint32(buf[8:12], 0)
	if _, _, _, err := proto.DecodeEnvelope(buf[:]); !errors.Is(err, proto.ErrPayloadTooLarge) {
		t.Errorf("DecodeEnvelope large: got %v, want ErrPayloadTooLarge", err)
	}
}

func TestEnvelopeLengthBelowHeader(t *testing.T) {
	var buf [proto.EnvelopeSize]byte
	binary.LittleEndian.PutUint32(buf[0:4], 3) // < HeaderSize
	if _, _, _, err := proto.DecodeEnvelope(buf[:]); !errors.Is(err, proto.ErrMalformed) {
		t.Errorf("DecodeEnvelope undersize: got %v, want ErrMalformed", err)
	}
}

func TestNewFrameUnknownType(t *testing.T) {
	_, err := proto.NewFrame(0xFFFF)
	if !errors.Is(err, proto.ErrUnknownType) {
		t.Fatalf("NewFrame(0xFFFF): got %v, want ErrUnknownType", err)
	}
	var fe *proto.FrameError
	if !errors.As(err, &fe) {
		t.Fatalf("NewFrame(0xFFFF): errors.As *FrameError failed on %v", err)
	}
	if fe.Op != proto.OpNewFrame {
		t.Errorf("FrameError.Op = %q, want %q", fe.Op, proto.OpNewFrame)
	}
	if fe.Type != 0xFFFF {
		t.Errorf("FrameError.Type = %#x, want 0xFFFF", uint16(fe.Type))
	}
}

func TestFrameErrorCarriesContext(t *testing.T) {
	// Malformed CommandList: force an unmarshal error and make sure
	// both the sentinel (Is) and the structured context (As) are
	// reachable.
	var p []byte
	p = binary.LittleEndian.AppendUint32(p, 0) // count=0 is invalid
	err := (&proto.CommandList{}).UnmarshalBinary(p)
	if !errors.Is(err, proto.ErrMalformed) {
		t.Fatalf("errors.Is ErrMalformed: err=%v", err)
	}
	var fe *proto.FrameError
	if !errors.As(err, &fe) {
		t.Fatalf("errors.As *FrameError: err=%v", err)
	}
	if fe.Op != proto.OpUnmarshal {
		t.Errorf("FrameError.Op = %q, want %q", fe.Op, proto.OpUnmarshal)
	}
	if fe.Type != proto.MsgCommandList {
		t.Errorf("FrameError.Type = %v, want %v", fe.Type, proto.MsgCommandList)
	}
	if fe.Detail == "" {
		t.Errorf("FrameError.Detail is empty; want a useful description")
	}
}

func TestIsServerToClient(t *testing.T) {
	clientBound := []proto.MsgType{
		proto.MsgIdentify, proto.MsgCommandList, proto.MsgInput,
		proto.MsgResize, proto.MsgCapsUpdate, proto.MsgBye,
	}
	serverBound := []proto.MsgType{
		proto.MsgOutput, proto.MsgCommandResult, proto.MsgExit, proto.MsgBeep,
	}
	for _, t2 := range clientBound {
		if t2.IsServerToClient() {
			t.Errorf("%v: IsServerToClient true, want false", t2)
		}
	}
	for _, t2 := range serverBound {
		if !t2.IsServerToClient() {
			t.Errorf("%v: IsServerToClient false, want true", t2)
		}
	}
}

func TestByeAndBeepRejectPayload(t *testing.T) {
	if err := (&proto.Bye{}).UnmarshalBinary([]byte{1}); !errors.Is(err, proto.ErrMalformed) {
		t.Errorf("Bye non-empty: got %v, want ErrMalformed", err)
	}
	if err := (&proto.Beep{}).UnmarshalBinary([]byte{1}); !errors.Is(err, proto.ErrMalformed) {
		t.Errorf("Beep non-empty: got %v, want ErrMalformed", err)
	}
}

func TestCommandListBounds(t *testing.T) {
	if _, err := (&proto.CommandList{}).MarshalBinary(); !errors.Is(err, proto.ErrMalformed) {
		t.Errorf("empty CommandList: got %v, want ErrMalformed", err)
	}

	too := make([]proto.Command, proto.MaxCommandsPerList+1)
	for i := range too {
		too[i] = proto.Command{ID: uint32(i), Argv: []string{"x"}}
	}
	if _, err := (&proto.CommandList{Commands: too}).MarshalBinary(); !errors.Is(err, proto.ErrMalformed) {
		t.Errorf("oversize CommandList: got %v, want ErrMalformed", err)
	}
}

func TestCommandResultRejectsUnknownStatus(t *testing.T) {
	// Hand-craft a payload with an out-of-range status byte.
	var w []byte
	w = binary.LittleEndian.AppendUint32(w, 42) // id
	w = append(w, 99)                           // status (invalid)
	w = binary.LittleEndian.AppendUint32(w, 0)  // empty message
	if err := (&proto.CommandResult{}).UnmarshalBinary(w); !errors.Is(err, proto.ErrMalformed) {
		t.Errorf("unknown status: got %v, want ErrMalformed", err)
	}
}

func TestExitRejectsUnknownReason(t *testing.T) {
	var w []byte
	w = append(w, 99)                          // reason (invalid)
	w = binary.LittleEndian.AppendUint32(w, 0) // empty message
	if err := (&proto.Exit{}).UnmarshalBinary(w); !errors.Is(err, proto.ErrMalformed) {
		t.Errorf("unknown reason: got %v, want ErrMalformed", err)
	}
}

func TestTrailingBytesRejected(t *testing.T) {
	payload, err := (&proto.Resize{Cols: 1, Rows: 2}).MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	payload = append(payload, 0xff)
	if err := (&proto.Resize{}).UnmarshalBinary(payload); !errors.Is(err, proto.ErrMalformed) {
		t.Errorf("trailing bytes: got %v, want ErrMalformed", err)
	}
}

func TestMsgTypeString(t *testing.T) {
	cases := map[proto.MsgType]string{
		proto.MsgIdentify:      "Identify",
		proto.MsgCommandList:   "CommandList",
		proto.MsgInput:         "Input",
		proto.MsgResize:        "Resize",
		proto.MsgCapsUpdate:    "CapsUpdate",
		proto.MsgBye:           "Bye",
		proto.MsgOutput:        "Output",
		proto.MsgCommandResult: "CommandResult",
		proto.MsgExit:          "Exit",
		proto.MsgBeep:          "Beep",
		0xABCD:                 "MsgType(0xabcd)",
	}
	for typ, want := range cases {
		if got := typ.String(); got != want {
			t.Errorf("MsgType(%#x).String() = %q, want %q", uint16(typ), got, want)
		}
	}
}

func TestCommandStatusString(t *testing.T) {
	cases := map[proto.CommandStatus]string{
		proto.StatusOk:      "ok",
		proto.StatusError:   "error",
		proto.StatusSkipped: "skipped",
		99:                  "CommandStatus(99)",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("CommandStatus(%d).String() = %q, want %q", uint8(s), got, want)
		}
	}
}

func TestExitReasonString(t *testing.T) {
	cases := map[proto.ExitReason]string{
		proto.ExitDetached:      "detached",
		proto.ExitDetachedOther: "detached-other",
		proto.ExitServerExit:    "server-exit",
		proto.ExitKilled:        "killed",
		proto.ExitProtocolError: "protocol-error",
		proto.ExitLost:          "lost",
		proto.ExitExitedShell:   "exited-shell",
		99:                      "ExitReason(99)",
	}
	for r, want := range cases {
		if got := r.String(); got != want {
			t.Errorf("ExitReason(%d).String() = %q, want %q", uint8(r), got, want)
		}
	}
}

func TestStringSliceAllocationCap(t *testing.T) {
	// Forged CommandList payload: outer count=1, command id=0,
	// argv count claims 1_000_000 with no argv bytes following.
	// Without the length-based cap, this forces a ~16 MiB
	// allocation before discovering EOF.
	var p []byte
	p = binary.LittleEndian.AppendUint32(p, 1)         // outer count
	p = binary.LittleEndian.AppendUint32(p, 0)         // command id
	p = binary.LittleEndian.AppendUint32(p, 1_000_000) // inner argv count
	// no argv bytes
	if err := (&proto.CommandList{}).UnmarshalBinary(p); !errors.Is(err, proto.ErrMalformed) {
		t.Errorf("oversize stringSlice: got %v, want ErrMalformed", err)
	}
}

func TestTruncatedPayload(t *testing.T) {
	full, err := (&proto.Identify{
		ProtocolVersion: 1, Profile: 0, InitialCols: 1, InitialRows: 1,
		Cwd: "abc", TTYName: "x", TermEnv: "y", Env: []string{}, Features: []uint8{},
	}).MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := (&proto.Identify{}).UnmarshalBinary(full[:len(full)-1]); err == nil {
		t.Error("truncated Identify: expected error, got nil")
	}
}
