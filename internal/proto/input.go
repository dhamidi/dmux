package proto

// Input carries raw stdin bytes from the user's terminal to the
// server. The payload is an unframed byte run; its length is the
// frame length minus header overhead.
type Input struct {
	Data []byte
}

func (*Input) Type() MsgType { return MsgInput }

// MarshalBinary returns m.Data directly. Callers must not mutate
// Data until the encoded bytes have been written to the wire.
func (m *Input) MarshalBinary() ([]byte, error) {
	return m.Data, nil
}

// UnmarshalBinary copies data so that m.Data does not alias a
// caller-owned buffer (xio reuses receive buffers across frames).
func (m *Input) UnmarshalBinary(data []byte) error {
	m.Data = append([]byte(nil), data...)
	return nil
}
