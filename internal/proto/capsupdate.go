package proto

// CapsUpdate delivers late-arriving capability information from the
// client (the DA2 and KKP probes complete asynchronously). Receiving
// it causes the server to rebuild this client's termin.Parser and
// termout.Renderer.
type CapsUpdate struct {
	Profile uint8
}

func (*CapsUpdate) Type() MsgType { return MsgCapsUpdate }

func (m *CapsUpdate) MarshalBinary() ([]byte, error) {
	var w bwriter
	w.u8(m.Profile)
	return w.bytes(), w.err
}

func (m *CapsUpdate) UnmarshalBinary(data []byte) error {
	r := breader{buf: data}
	m.Profile = r.u8()
	return r.finish()
}
