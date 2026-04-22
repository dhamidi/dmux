package proto

import "fmt"

// Beep notifies the client that the focused pane wants to ring the
// bell. Empty payload.
type Beep struct{}

func (*Beep) Type() MsgType { return MsgBeep }

func (*Beep) MarshalBinary() ([]byte, error) { return nil, nil }

func (*Beep) UnmarshalBinary(data []byte) error {
	if len(data) != 0 {
		return fmt.Errorf("%w: Beep payload must be empty, got %d bytes", ErrMalformed, len(data))
	}
	return nil
}
