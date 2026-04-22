package proto

// MaxCommandsPerList caps the number of commands in a single
// CommandList. Real bootstraps send 1-3; 64 is a defensive ceiling.
const MaxCommandsPerList = 64

// Command is one argv entry inside a CommandList.
type Command struct {
	// ID is a client-assigned correlation number used to match this
	// command to its CommandResult. It is opaque to the server: any
	// value the client picks is echoed back verbatim. IDs must be
	// unique within one CommandList; they need not be monotonic and
	// have no meaning across CommandLists.
	//
	// ID does not identify *which* command to run — that is the
	// first element of Argv. New commands are added by defining a
	// new name (e.g. "split-window"), not by extending this field.
	ID uint32

	// Argv is the command and its arguments in shell-style form,
	// e.g. {"new-session", "-d", "-s", "work"}. Argv[0] is the
	// command name; the server looks it up in the cmd registry.
	Argv []string
}

// CommandList is an ordered group of commands executed as a unit.
// If a command returns error, subsequent commands in the same list
// are skipped (status StatusSkipped in their CommandResult). This
// gives the bootstrap "new-session; attach-session" fail-open
// semantics without an ack-per-command round trip.
type CommandList struct {
	Commands []Command
}

func (*CommandList) Type() MsgType { return MsgCommandList }

func (m *CommandList) MarshalBinary() ([]byte, error) {
	n := len(m.Commands)
	if n == 0 || n > MaxCommandsPerList {
		return nil, frameErr(OpMarshal, MsgCommandList, ErrMalformed, "command count %d (want 1..%d)", n, MaxCommandsPerList)
	}
	w := bwriter{op: OpMarshal, typ: MsgCommandList}
	w.u32(uint32(n))
	for _, c := range m.Commands {
		w.u32(c.ID)
		w.stringSlice(c.Argv)
	}
	return w.bytes(), w.err
}

func (m *CommandList) UnmarshalBinary(data []byte) error {
	r := breader{op: OpUnmarshal, typ: MsgCommandList, buf: data}
	n := r.u32()
	if r.err != nil {
		return r.err
	}
	if n == 0 || n > MaxCommandsPerList {
		return frameErr(OpUnmarshal, MsgCommandList, ErrMalformed, "command count %d (want 1..%d)", n, MaxCommandsPerList)
	}
	m.Commands = make([]Command, n)
	for i := range m.Commands {
		m.Commands[i].ID = r.u32()
		m.Commands[i].Argv = r.stringSlice()
		if r.err != nil {
			return r.err
		}
	}
	return r.finish()
}
