package proto

// Identify is the first frame every client sends. The server
// rejects any other frame before it with Exit{ProtocolError}.
//
// Profile holds a termcaps profile id on the wire (Unknown=0,
// Ghostty=1, XTermJSModern=2, XTermJSLegacy=3, WindowsTerminal=4).
// The termcaps package owns the typed enum; proto keeps the u8 so
// this package sits below termcaps in the dependency order.
type Identify struct {
	ProtocolVersion uint8
	Profile         uint8
	InitialCols     uint32
	InitialRows     uint32
	Cwd             string
	TTYName         string
	TermEnv         string
	Env             []string
	Features        []uint8
}

func (*Identify) Type() MsgType { return MsgIdentify }

func (m *Identify) MarshalBinary() ([]byte, error) {
	if len(m.Features) > 0xFF {
		return nil, frameErr(OpMarshal, MsgIdentify, ErrPayloadTooLarge, "features count %d > 255", len(m.Features))
	}
	w := bwriter{op: OpMarshal, typ: MsgIdentify}
	w.u8(m.ProtocolVersion)
	w.u8(m.Profile)
	w.u32(m.InitialCols)
	w.u32(m.InitialRows)
	w.string(m.Cwd)
	w.string(m.TTYName)
	w.string(m.TermEnv)
	w.stringSlice(m.Env)
	w.u8(uint8(len(m.Features)))
	for _, f := range m.Features {
		w.u8(f)
	}
	return w.bytes(), w.err
}

func (m *Identify) UnmarshalBinary(data []byte) error {
	r := breader{op: OpUnmarshal, typ: MsgIdentify, buf: data}
	m.ProtocolVersion = r.u8()
	m.Profile = r.u8()
	m.InitialCols = r.u32()
	m.InitialRows = r.u32()
	m.Cwd = r.string()
	m.TTYName = r.string()
	m.TermEnv = r.string()
	m.Env = r.stringSlice()
	n := r.u8()
	if r.err != nil {
		return r.err
	}
	m.Features = make([]uint8, n)
	for i := range m.Features {
		m.Features[i] = r.u8()
	}
	return r.finish()
}
