package proto

// MsgType identifies the kind of a framed wire message.
type MsgType uint32

const (
	// Identify category — sent by the client on attach so the server knows
	// what terminal capabilities to use and where to spawn new panes.
	MsgIdentifyFlags     MsgType = 1
	MsgIdentifyTerm      MsgType = 2
	MsgIdentifyTerminfo  MsgType = 3
	MsgIdentifyTTYName   MsgType = 4
	MsgIdentifyCWD       MsgType = 5
	MsgIdentifyEnviron   MsgType = 6
	MsgIdentifyClientPID MsgType = 7
	MsgIdentifyFeatures  MsgType = 8
	MsgIdentifyDone      MsgType = 9

	// Session category — normal attached-client lifecycle.
	MsgVersion  MsgType = 100
	MsgCommand  MsgType = 101
	MsgResize   MsgType = 102
	MsgDetach   MsgType = 103
	MsgExit     MsgType = 104
	MsgExited   MsgType = 105
	MsgShutdown MsgType = 106

	// Data category — raw bytes flowing between client terminal and server
	// rendered output.
	MsgStdin  MsgType = 200
	MsgStdout MsgType = 201
	MsgStderr MsgType = 202

	// File RPC category — lets the server ask the client to read from or
	// write to a file the client has access to.
	MsgReadOpen   MsgType = 300
	MsgRead       MsgType = 301
	MsgReadDone   MsgType = 302
	MsgWriteOpen  MsgType = 303
	MsgWrite      MsgType = 304
	MsgWriteReady MsgType = 305
	MsgWriteClose MsgType = 306
)

// String returns the human-readable name of a MsgType.
func (t MsgType) String() string {
	switch t {
	case MsgIdentifyFlags:
		return "IDENTIFY_FLAGS"
	case MsgIdentifyTerm:
		return "IDENTIFY_TERM"
	case MsgIdentifyTerminfo:
		return "IDENTIFY_TERMINFO"
	case MsgIdentifyTTYName:
		return "IDENTIFY_TTYNAME"
	case MsgIdentifyCWD:
		return "IDENTIFY_CWD"
	case MsgIdentifyEnviron:
		return "IDENTIFY_ENVIRON"
	case MsgIdentifyClientPID:
		return "IDENTIFY_CLIENTPID"
	case MsgIdentifyFeatures:
		return "IDENTIFY_FEATURES"
	case MsgIdentifyDone:
		return "IDENTIFY_DONE"
	case MsgVersion:
		return "VERSION"
	case MsgCommand:
		return "COMMAND"
	case MsgResize:
		return "RESIZE"
	case MsgDetach:
		return "DETACH"
	case MsgExit:
		return "EXIT"
	case MsgExited:
		return "EXITED"
	case MsgShutdown:
		return "SHUTDOWN"
	case MsgStdin:
		return "STDIN"
	case MsgStdout:
		return "STDOUT"
	case MsgStderr:
		return "STDERR"
	case MsgReadOpen:
		return "READ_OPEN"
	case MsgRead:
		return "READ"
	case MsgReadDone:
		return "READ_DONE"
	case MsgWriteOpen:
		return "WRITE_OPEN"
	case MsgWrite:
		return "WRITE"
	case MsgWriteReady:
		return "WRITE_READY"
	case MsgWriteClose:
		return "WRITE_CLOSE"
	default:
		return "UNKNOWN"
	}
}
