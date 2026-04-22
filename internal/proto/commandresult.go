package proto

import "fmt"

// CommandStatus is the outcome of a single Command in a CommandList.
type CommandStatus uint8

const (
	StatusOk      CommandStatus = 0
	StatusError   CommandStatus = 1
	StatusSkipped CommandStatus = 2
)

func (s CommandStatus) String() string {
	switch s {
	case StatusOk:
		return "ok"
	case StatusError:
		return "error"
	case StatusSkipped:
		return "skipped"
	default:
		return fmt.Sprintf("CommandStatus(%d)", uint8(s))
	}
}

// CommandResult is the server's reply to one Command inside a
// CommandList. The server emits one per Command in arrival order.
// Status=StatusSkipped means an earlier Command in the same list
// returned StatusError.
type CommandResult struct {
	// ID echoes the Command.ID it is answering. The client uses
	// this to correlate replies with commands when the list has
	// more than one entry.
	ID uint32

	// Status is the outcome: ok, error, or skipped (when a prior
	// command in the same list returned error).
	Status CommandStatus

	// Message is a human-readable error or info string. Empty on
	// success; formatted error text on StatusError; an explanatory
	// "skipped: <prior command> failed" on StatusSkipped.
	Message string
}

func (*CommandResult) Type() MsgType { return MsgCommandResult }

func (m *CommandResult) MarshalBinary() ([]byte, error) {
	var w bwriter
	w.u32(m.ID)
	w.u8(uint8(m.Status))
	w.string(m.Message)
	return w.bytes(), w.err
}

func (m *CommandResult) UnmarshalBinary(data []byte) error {
	r := breader{buf: data}
	m.ID = r.u32()
	raw := r.u8()
	m.Status = CommandStatus(raw)
	m.Message = r.string()
	if err := r.finish(); err != nil {
		return err
	}
	switch m.Status {
	case StatusOk, StatusError, StatusSkipped:
		return nil
	default:
		return fmt.Errorf("%w: command status %d", ErrMalformed, raw)
	}
}
