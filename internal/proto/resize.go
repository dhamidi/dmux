package proto

// Resize tells the server that the client's terminal window changed
// size. Cols and Rows are cell counts, not pixels.
type Resize struct {
	Cols uint32
	Rows uint32
}

func (*Resize) Type() MsgType { return MsgResize }

func (m *Resize) MarshalBinary() ([]byte, error) {
	w := bwriter{op: OpMarshal, typ: MsgResize}
	w.u32(m.Cols)
	w.u32(m.Rows)
	return w.bytes(), w.err
}

func (m *Resize) UnmarshalBinary(data []byte) error {
	r := breader{op: OpUnmarshal, typ: MsgResize, buf: data}
	m.Cols = r.u32()
	m.Rows = r.u32()
	return r.finish()
}
