package proto

// Output carries bytes the client should write to its real
// terminal stdout. The payload is an unframed byte run; its length
// is the frame length minus header overhead.
type Output struct {
	Data []byte
}

func (*Output) Type() MsgType { return MsgOutput }

// MarshalBinary returns m.Data directly. Callers must not mutate
// Data until the encoded bytes have been written to the wire.
func (m *Output) MarshalBinary() ([]byte, error) {
	return m.Data, nil
}

// UnmarshalBinary copies data so that m.Data does not alias a
// caller-owned buffer.
func (m *Output) UnmarshalBinary(data []byte) error {
	m.Data = append([]byte(nil), data...)
	return nil
}
