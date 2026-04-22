package client

import (
	"fmt"
	"io"
	"os"

	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/term"
)

// protoVersion is the wire protocol version this client implements.
const protoVersion uint16 = 1

// Client-flag bits sent in IDENTIFY_FLAGS.
const (
	// FlagControlMode enables machine-readable control-mode output (flag -CC).
	FlagControlMode uint32 = 1 << 0
	// FlagReadOnly prevents the client from forwarding stdin to the server.
	FlagReadOnly uint32 = 1 << 1
)

// Config holds the injected dependencies and settings for the dmux client.
// All I/O is explicit so the client is testable without a real terminal.
type Config struct {
	// Conn is the open connection to the server. Run communicates with the
	// server exclusively through this ReadWriter using the proto framing
	// layer. The caller (typically cmd/dmux) is responsible for dialling
	// cfg.SocketPath and assigning the resulting net.Conn here.
	Conn io.ReadWriter

	// SocketPath is the path to the server's Unix-domain socket. Used by
	// cmd/ to dial before constructing Config; Run does not use it directly.
	SocketPath string

	// In is the source of raw terminal input bytes (typically os.Stdin or a
	// TTY file). May be nil when ReadOnly is true.
	In io.Reader

	// Out is the destination for terminal output written by the server
	// (typically os.Stdout or the same TTY file).
	Out io.Writer

	// Size queries the current terminal dimensions. Use [term.OSSize] for a
	// real terminal; supply a stub returning fixed dimensions in tests.
	Size term.SizeFunc

	// RawMode enters raw mode on the controlling terminal and returns a
	// restore function. Use [term.OSRawMode] for a real terminal; may be
	// nil in tests.
	RawMode term.RawModeFunc

	// Argv is the command to send to the server as MSG_COMMAND, if any.
	Argv []string

	// ControlMode enables machine-readable control-mode output (-CC flag).
	ControlMode bool

	// ReadOnly prevents the client from sending stdin input to the server.
	ReadOnly bool
}

// NewOSConfig returns a Config wired to the process's real terminal.
// f is the controlling terminal file (typically os.Stdin or /dev/tty);
// out is the output writer (typically os.Stdout).
// Conn must be set by the caller after dialling cfg.SocketPath.
func NewOSConfig(socketPath string, f *os.File, out io.Writer) Config {
	return Config{
		SocketPath: socketPath,
		In:         f,
		Out:        out,
		Size:       term.OSSize(f),
		RawMode:    term.OSRawMode(f),
	}
}

// Run connects to the dmux server using cfg.Conn and runs the client
// forwarding loop. It returns the process exit code. The terminal is put
// in raw mode (via cfg.RawMode) for the duration of the session and
// restored on exit.
//
// Run never accesses os.Stdin or os.Stdout directly; all I/O flows
// through the fields of cfg.
func Run(cfg Config) int {
	conn := cfg.Conn

	// Send VERSION first so the server can reject incompatible clients early.
	vm := proto.VersionMsg{Version: protoVersion}
	if err := proto.WriteMsg(conn, proto.MsgVersion, vm.Encode()); err != nil {
		fmt.Fprintf(os.Stderr, "dmux: write version: %v\n", err)
		return 1
	}

	// Send the full IDENTIFY sequence so the server knows our capabilities.
	if err := sendIdentify(conn, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "dmux: identify: %v\n", err)
		return 1
	}

	// Optionally send the initial command.
	if len(cfg.Argv) > 0 {
		cm := proto.CommandMsg{Argv: cfg.Argv}
		if err := proto.WriteMsg(conn, proto.MsgCommand, cm.Encode()); err != nil {
			fmt.Fprintf(os.Stderr, "dmux: write command: %v\n", err)
			return 1
		}
	}

	// Enter raw mode if configured.
	if cfg.RawMode != nil {
		restore, err := cfg.RawMode()
		if err != nil {
			fmt.Fprintf(os.Stderr, "dmux: raw mode: %v\n", err)
			return 1
		}
		defer restore()
	}

	// Switch to the alternate screen buffer so that the user's scrollback
	// is preserved when dmux exits.
	if cfg.Out != nil {
		fmt.Fprint(cfg.Out, "\x1b[?1049h")
		defer fmt.Fprint(cfg.Out, "\x1b[?1049l")
	}

	// Pump stdin → server in a background goroutine.
	if !cfg.ReadOnly && cfg.In != nil {
		go pumpStdin(conn, cfg.In)
	}

	// Server → stdout loop; returns on EXIT message or connection close.
	return serverLoop(conn, cfg.Out)
}

// sendIdentify writes the full IDENTIFY_* sequence followed by RESIZE to w.
// It reads environment variables and process info directly from the OS,
// which is acceptable since they are not terminal I/O.
func sendIdentify(w io.Writer, cfg Config) error {
	var flags uint32
	if cfg.ControlMode {
		flags |= FlagControlMode
	}
	if cfg.ReadOnly {
		flags |= FlagReadOnly
	}

	cwd, _ := os.Getwd()

	msgs := []struct {
		t proto.MsgType
		p []byte
	}{
		{proto.MsgIdentifyFlags, (proto.IdentifyFlagsMsg{Flags: flags}).Encode()},
		{proto.MsgIdentifyTerm, (proto.IdentifyTermMsg{Term: os.Getenv("TERM")}).Encode()},
		{proto.MsgIdentifyCWD, (proto.IdentifyCWDMsg{CWD: cwd}).Encode()},
		{proto.MsgIdentifyEnviron, (proto.IdentifyEnvironMsg{Pairs: os.Environ()}).Encode()},
		{proto.MsgIdentifyClientPID, (proto.IdentifyClientPIDMsg{PID: int32(os.Getpid())}).Encode()},
		{proto.MsgIdentifyFeatures, (proto.IdentifyFeaturesMsg{Features: 0}).Encode()},
		{proto.MsgIdentifyDone, (proto.IdentifyDoneMsg{}).Encode()},
	}
	for _, m := range msgs {
		if err := proto.WriteMsg(w, m.t, m.p); err != nil {
			return err
		}
	}

	// Report initial terminal dimensions after IDENTIFY_DONE.
	if cfg.Size != nil {
		rows, cols, err := cfg.Size()
		if err == nil {
			rm := proto.ResizeMsg{Width: uint16(cols), Height: uint16(rows)}
			return proto.WriteMsg(w, proto.MsgResize, rm.Encode())
		}
	}
	return nil
}

// pumpStdin reads raw bytes from in and forwards them as STDIN messages to
// conn. It returns when in is exhausted or conn write fails.
func pumpStdin(conn io.Writer, in io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			m := proto.StdinMsg{Data: buf[:n]}
			if werr := proto.WriteMsg(conn, proto.MsgStdin, m.Encode()); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// serverLoop reads framed messages from conn and dispatches them until an
// EXIT message is received or the connection is closed.
func serverLoop(conn io.ReadWriter, out io.Writer) int {
	for {
		msgType, payload, err := proto.ReadMsg(conn)
		if err != nil {
			// EOF or connection reset → clean server shutdown.
			return 0
		}
		switch msgType {
		case proto.MsgStdout:
			var m proto.StdoutMsg
			if m.Decode(payload) == nil {
				_, _ = out.Write(m.Data)
			}
		case proto.MsgStderr:
			var m proto.StderrMsg
			if m.Decode(payload) == nil {
				_, _ = out.Write(m.Data)
			}
		case proto.MsgExit:
			var m proto.ExitMsg
			if m.Decode(payload) == nil {
				return int(m.Code)
			}
			return 1
		case proto.MsgReadOpen:
			var m proto.ReadOpenMsg
			if m.Decode(payload) == nil {
				fileReadRPC(conn, m.Path)
			}
		case proto.MsgWriteOpen:
			var m proto.WriteOpenMsg
			if m.Decode(payload) == nil {
				fileWriteRPC(conn, m.Path)
			}
		}
	}
}

// fileReadRPC handles a READ_OPEN request: it opens the named file and
// streams its contents as READ messages, finishing with READ_DONE.
func fileReadRPC(conn io.ReadWriter, path string) {
	f, err := os.Open(path)
	if err == nil {
		buf := make([]byte, 32*1024)
		for {
			n, rerr := f.Read(buf)
			if n > 0 {
				m := proto.FileReadMsg{Data: buf[:n]}
				if werr := proto.WriteMsg(conn, proto.MsgRead, m.Encode()); werr != nil {
					f.Close()
					return
				}
			}
			if rerr != nil {
				break
			}
		}
		f.Close()
	}
	_ = proto.WriteMsg(conn, proto.MsgReadDone, (proto.ReadDoneMsg{}).Encode())
}

// fileWriteRPC handles a WRITE_OPEN request: it sends WRITE_READY and then
// receives WRITE messages, writing each chunk to the named file, until
// WRITE_CLOSE is received.
func fileWriteRPC(conn io.ReadWriter, path string) {
	if err := proto.WriteMsg(conn, proto.MsgWriteReady, (proto.WriteReadyMsg{}).Encode()); err != nil {
		return
	}

	f, ferr := os.Create(path)
	for {
		t, payload, err := proto.ReadMsg(conn)
		if err != nil {
			break
		}
		if t == proto.MsgWriteClose {
			break
		}
		if t == proto.MsgWrite && ferr == nil {
			var m proto.FileWriteMsg
			if m.Decode(payload) == nil {
				_, _ = f.Write(m.Data)
			}
		}
	}
	if ferr == nil {
		f.Close()
	}
}
