// Package proto codec — per-message-type encode/decode helpers.
//
// Each message type has a corresponding Go struct with only primitive or
// stdlib fields (no internal/* dependencies). Encode produces a payload
// []byte suitable for WriteMsg; Decode parses a payload []byte returned
// by ReadMsg.
//
// Payload encoding conventions:
//
//   - string:   4-byte big-endian length followed by UTF-8 bytes.
//   - []string: 4-byte big-endian count, then each string as above.
//   - uint16:   2 bytes big-endian.
//   - uint32:   4 bytes big-endian.
//   - int32:    4 bytes big-endian (cast to/from uint32).
//   - []byte:   raw bytes occupying the entire payload.
//   - empty:    zero-length payload.

package proto

import (
	"encoding/binary"
	"fmt"
)

// ---- helpers ----------------------------------------------------------------

func putUint16(b []byte, v uint16) { binary.BigEndian.PutUint16(b, v) }
func putUint32(b []byte, v uint32) { binary.BigEndian.PutUint32(b, v) }

func getUint16(b []byte) uint16 { return binary.BigEndian.Uint16(b) }
func getUint32(b []byte) uint32 { return binary.BigEndian.Uint32(b) }

// encodeString encodes a single string into a new []byte.
func encodeString(s string) []byte {
	out := make([]byte, 4+len(s))
	putUint32(out, uint32(len(s)))
	copy(out[4:], s)
	return out
}

// encodeStrings encodes a []string into a new []byte.
func encodeStrings(ss []string) []byte {
	size := 4
	for _, s := range ss {
		size += 4 + len(s)
	}
	out := make([]byte, size)
	putUint32(out, uint32(len(ss)))
	off := 4
	for _, s := range ss {
		putUint32(out[off:], uint32(len(s)))
		off += 4
		copy(out[off:], s)
		off += len(s)
	}
	return out
}

// decodeString reads a length-prefixed string from p starting at off.
// Returns the string and the new offset, or an error.
func decodeString(p []byte, off int) (string, int, error) {
	if off+4 > len(p) {
		return "", off, fmt.Errorf("proto: truncated string length at offset %d", off)
	}
	n := int(getUint32(p[off:]))
	off += 4
	if off+n > len(p) {
		return "", off, fmt.Errorf("proto: truncated string data at offset %d (need %d bytes)", off, n)
	}
	s := string(p[off : off+n])
	return s, off + n, nil
}

// decodeStrings reads a count-prefixed string slice from p starting at off.
func decodeStrings(p []byte, off int) ([]string, int, error) {
	if off+4 > len(p) {
		return nil, off, fmt.Errorf("proto: truncated string count at offset %d", off)
	}
	count := int(getUint32(p[off:]))
	off += 4
	ss := make([]string, count)
	for i := 0; i < count; i++ {
		var err error
		ss[i], off, err = decodeString(p, off)
		if err != nil {
			return nil, off, err
		}
	}
	return ss, off, nil
}

// ---- Identify category ------------------------------------------------------

// IdentifyFlagsMsg carries a bitmask of client flags.
type IdentifyFlagsMsg struct {
	Flags uint32
}

func (m IdentifyFlagsMsg) Encode() []byte {
	b := make([]byte, 4)
	putUint32(b, m.Flags)
	return b
}

func (m *IdentifyFlagsMsg) Decode(p []byte) error {
	if len(p) < 4 {
		return fmt.Errorf("proto: IDENTIFY_FLAGS payload too short")
	}
	m.Flags = getUint32(p)
	return nil
}

// IdentifyTermMsg carries the TERM environment variable value.
type IdentifyTermMsg struct {
	Term string
}

func (m IdentifyTermMsg) Encode() []byte { return encodeString(m.Term) }

func (m *IdentifyTermMsg) Decode(p []byte) error {
	s, _, err := decodeString(p, 0)
	if err != nil {
		return fmt.Errorf("proto: IDENTIFY_TERM: %w", err)
	}
	m.Term = s
	return nil
}

// IdentifyTerminfoMsg carries raw terminfo data bytes.
type IdentifyTerminfoMsg struct {
	Data []byte
}

func (m IdentifyTerminfoMsg) Encode() []byte {
	out := make([]byte, len(m.Data))
	copy(out, m.Data)
	return out
}

func (m *IdentifyTerminfoMsg) Decode(p []byte) error {
	m.Data = make([]byte, len(p))
	copy(m.Data, p)
	return nil
}

// IdentifyTTYNameMsg carries the path of the client's controlling TTY.
type IdentifyTTYNameMsg struct {
	TTYName string
}

func (m IdentifyTTYNameMsg) Encode() []byte { return encodeString(m.TTYName) }

func (m *IdentifyTTYNameMsg) Decode(p []byte) error {
	s, _, err := decodeString(p, 0)
	if err != nil {
		return fmt.Errorf("proto: IDENTIFY_TTYNAME: %w", err)
	}
	m.TTYName = s
	return nil
}

// IdentifyCWDMsg carries the client's current working directory.
type IdentifyCWDMsg struct {
	CWD string
}

func (m IdentifyCWDMsg) Encode() []byte { return encodeString(m.CWD) }

func (m *IdentifyCWDMsg) Decode(p []byte) error {
	s, _, err := decodeString(p, 0)
	if err != nil {
		return fmt.Errorf("proto: IDENTIFY_CWD: %w", err)
	}
	m.CWD = s
	return nil
}

// IdentifyEnvironMsg carries the client's environment as "KEY=VALUE" strings.
type IdentifyEnvironMsg struct {
	Pairs []string
}

func (m IdentifyEnvironMsg) Encode() []byte { return encodeStrings(m.Pairs) }

func (m *IdentifyEnvironMsg) Decode(p []byte) error {
	ss, _, err := decodeStrings(p, 0)
	if err != nil {
		return fmt.Errorf("proto: IDENTIFY_ENVIRON: %w", err)
	}
	m.Pairs = ss
	return nil
}

// IdentifyClientPIDMsg carries the client process ID.
type IdentifyClientPIDMsg struct {
	PID int32
}

func (m IdentifyClientPIDMsg) Encode() []byte {
	b := make([]byte, 4)
	putUint32(b, uint32(m.PID))
	return b
}

func (m *IdentifyClientPIDMsg) Decode(p []byte) error {
	if len(p) < 4 {
		return fmt.Errorf("proto: IDENTIFY_CLIENTPID payload too short")
	}
	m.PID = int32(getUint32(p))
	return nil
}

// IdentifyFeaturesMsg carries a bitmask of client feature flags.
type IdentifyFeaturesMsg struct {
	Features uint32
}

func (m IdentifyFeaturesMsg) Encode() []byte {
	b := make([]byte, 4)
	putUint32(b, m.Features)
	return b
}

func (m *IdentifyFeaturesMsg) Decode(p []byte) error {
	if len(p) < 4 {
		return fmt.Errorf("proto: IDENTIFY_FEATURES payload too short")
	}
	m.Features = getUint32(p)
	return nil
}

// IdentifyDoneMsg signals the end of the identify sequence. No payload.
type IdentifyDoneMsg struct{}

func (m IdentifyDoneMsg) Encode() []byte    { return nil }
func (m *IdentifyDoneMsg) Decode(_ []byte) error { return nil }

// ---- Session category -------------------------------------------------------

// VersionMsg carries the protocol version number. It is the first message sent
// on every connection. Incompatible versions are rejected with EXIT.
type VersionMsg struct {
	Version uint16
}

func (m VersionMsg) Encode() []byte {
	b := make([]byte, 2)
	putUint16(b, m.Version)
	return b
}

func (m *VersionMsg) Decode(p []byte) error {
	if len(p) < 2 {
		return fmt.Errorf("proto: VERSION payload too short")
	}
	m.Version = getUint16(p)
	return nil
}

// CommandMsg carries a command and its arguments as sent by the client.
type CommandMsg struct {
	Argv []string
}

func (m CommandMsg) Encode() []byte { return encodeStrings(m.Argv) }

func (m *CommandMsg) Decode(p []byte) error {
	ss, _, err := decodeStrings(p, 0)
	if err != nil {
		return fmt.Errorf("proto: COMMAND: %w", err)
	}
	m.Argv = ss
	return nil
}

// ResizeMsg carries the new terminal dimensions reported by the client.
type ResizeMsg struct {
	Width  uint16
	Height uint16
}

func (m ResizeMsg) Encode() []byte {
	b := make([]byte, 4)
	putUint16(b[0:], m.Width)
	putUint16(b[2:], m.Height)
	return b
}

func (m *ResizeMsg) Decode(p []byte) error {
	if len(p) < 4 {
		return fmt.Errorf("proto: RESIZE payload too short")
	}
	m.Width = getUint16(p[0:])
	m.Height = getUint16(p[2:])
	return nil
}

// DetachMsg requests that the server detach this client. No payload.
type DetachMsg struct{}

func (m DetachMsg) Encode() []byte       { return nil }
func (m *DetachMsg) Decode(_ []byte) error { return nil }

// ExitMsg is sent by the server to tell the client to exit with Code.
type ExitMsg struct {
	Code int32
}

func (m ExitMsg) Encode() []byte {
	b := make([]byte, 4)
	putUint32(b, uint32(m.Code))
	return b
}

func (m *ExitMsg) Decode(p []byte) error {
	if len(p) < 4 {
		return fmt.Errorf("proto: EXIT payload too short")
	}
	m.Code = int32(getUint32(p))
	return nil
}

// ExitedMsg is sent by the server after a child process has exited.
type ExitedMsg struct {
	Code int32
}

func (m ExitedMsg) Encode() []byte {
	b := make([]byte, 4)
	putUint32(b, uint32(m.Code))
	return b
}

func (m *ExitedMsg) Decode(p []byte) error {
	if len(p) < 4 {
		return fmt.Errorf("proto: EXITED payload too short")
	}
	m.Code = int32(getUint32(p))
	return nil
}

// ShutdownMsg requests or confirms a server shutdown. No payload.
type ShutdownMsg struct{}

func (m ShutdownMsg) Encode() []byte        { return nil }
func (m *ShutdownMsg) Decode(_ []byte) error { return nil }

// ---- Data category ----------------------------------------------------------

// StdinMsg carries raw bytes from the client's terminal input.
type StdinMsg struct {
	Data []byte
}

func (m StdinMsg) Encode() []byte {
	out := make([]byte, len(m.Data))
	copy(out, m.Data)
	return out
}

func (m *StdinMsg) Decode(p []byte) error {
	m.Data = make([]byte, len(p))
	copy(m.Data, p)
	return nil
}

// StdoutMsg carries rendered output bytes for the client's terminal.
type StdoutMsg struct {
	Data []byte
}

func (m StdoutMsg) Encode() []byte {
	out := make([]byte, len(m.Data))
	copy(out, m.Data)
	return out
}

func (m *StdoutMsg) Decode(p []byte) error {
	m.Data = make([]byte, len(p))
	copy(m.Data, p)
	return nil
}

// StderrMsg carries error output bytes for the client's terminal.
type StderrMsg struct {
	Data []byte
}

func (m StderrMsg) Encode() []byte {
	out := make([]byte, len(m.Data))
	copy(out, m.Data)
	return out
}

func (m *StderrMsg) Decode(p []byte) error {
	m.Data = make([]byte, len(p))
	copy(m.Data, p)
	return nil
}

// ---- File RPC category ------------------------------------------------------

// ReadOpenMsg asks the client to open a file for reading. The server sends this;
// the client responds with READ messages followed by READ_DONE.
type ReadOpenMsg struct {
	Path string
}

func (m ReadOpenMsg) Encode() []byte { return encodeString(m.Path) }

func (m *ReadOpenMsg) Decode(p []byte) error {
	s, _, err := decodeString(p, 0)
	if err != nil {
		return fmt.Errorf("proto: READ_OPEN: %w", err)
	}
	m.Path = s
	return nil
}

// FileReadMsg carries a chunk of file data from the client back to the server.
type FileReadMsg struct {
	Data []byte
}

func (m FileReadMsg) Encode() []byte {
	out := make([]byte, len(m.Data))
	copy(out, m.Data)
	return out
}

func (m *FileReadMsg) Decode(p []byte) error {
	m.Data = make([]byte, len(p))
	copy(m.Data, p)
	return nil
}

// ReadDoneMsg signals the end of a read file-RPC sequence. No payload.
type ReadDoneMsg struct{}

func (m ReadDoneMsg) Encode() []byte        { return nil }
func (m *ReadDoneMsg) Decode(_ []byte) error { return nil }

// WriteOpenMsg asks the client to open a file for writing. The server sends
// this; the client responds with WRITE_READY and then accepts WRITE messages.
type WriteOpenMsg struct {
	Path string
}

func (m WriteOpenMsg) Encode() []byte { return encodeString(m.Path) }

func (m *WriteOpenMsg) Decode(p []byte) error {
	s, _, err := decodeString(p, 0)
	if err != nil {
		return fmt.Errorf("proto: WRITE_OPEN: %w", err)
	}
	m.Path = s
	return nil
}

// FileWriteMsg carries a chunk of file data from the server to the client.
type FileWriteMsg struct {
	Data []byte
}

func (m FileWriteMsg) Encode() []byte {
	out := make([]byte, len(m.Data))
	copy(out, m.Data)
	return out
}

func (m *FileWriteMsg) Decode(p []byte) error {
	m.Data = make([]byte, len(p))
	copy(m.Data, p)
	return nil
}

// WriteReadyMsg signals that the client is ready to receive WRITE messages.
// No payload.
type WriteReadyMsg struct{}

func (m WriteReadyMsg) Encode() []byte        { return nil }
func (m *WriteReadyMsg) Decode(_ []byte) error { return nil }

// WriteCloseMsg signals the end of a write file-RPC sequence. No payload.
type WriteCloseMsg struct{}

func (m WriteCloseMsg) Encode() []byte        { return nil }
func (m *WriteCloseMsg) Decode(_ []byte) error { return nil }
