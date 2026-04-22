package proto

import "fmt"

// Bye signals a clean-detach intent so the server does not log the
// connection drop as abnormal. The server responds with
// Exit{Detached}. Empty payload.
type Bye struct{}

func (*Bye) Type() MsgType { return MsgBye }

func (*Bye) MarshalBinary() ([]byte, error) { return nil, nil }

func (*Bye) UnmarshalBinary(data []byte) error {
	if len(data) != 0 {
		return fmt.Errorf("%w: Bye payload must be empty, got %d bytes", ErrMalformed, len(data))
	}
	return nil
}
