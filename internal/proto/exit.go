package proto

import "fmt"

// ExitReason enumerates why the server is closing a connection.
type ExitReason uint8

const (
	ExitDetached      ExitReason = 0
	ExitDetachedOther ExitReason = 1
	ExitServerExit    ExitReason = 2
	ExitKilled        ExitReason = 3
	ExitProtocolError ExitReason = 4
	ExitLost          ExitReason = 5
	ExitExitedShell   ExitReason = 6
)

func (r ExitReason) String() string {
	switch r {
	case ExitDetached:
		return "detached"
	case ExitDetachedOther:
		return "detached-other"
	case ExitServerExit:
		return "server-exit"
	case ExitKilled:
		return "killed"
	case ExitProtocolError:
		return "protocol-error"
	case ExitLost:
		return "lost"
	case ExitExitedShell:
		return "exited-shell"
	default:
		return fmt.Sprintf("ExitReason(%d)", uint8(r))
	}
}

// Exit is the last frame the server sends on a connection.
type Exit struct {
	Reason  ExitReason
	Message string
}

func (*Exit) Type() MsgType { return MsgExit }

func (m *Exit) MarshalBinary() ([]byte, error) {
	var w bwriter
	w.u8(uint8(m.Reason))
	w.string(m.Message)
	return w.bytes(), w.err
}

func (m *Exit) UnmarshalBinary(data []byte) error {
	r := breader{buf: data}
	raw := r.u8()
	m.Reason = ExitReason(raw)
	m.Message = r.string()
	if err := r.finish(); err != nil {
		return err
	}
	switch m.Reason {
	case ExitDetached, ExitDetachedOther, ExitServerExit, ExitKilled,
		ExitProtocolError, ExitLost, ExitExitedShell:
		return nil
	default:
		return fmt.Errorf("%w: exit reason %d", ErrMalformed, raw)
	}
}
