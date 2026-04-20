package proto_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/dhamidi/dmux/internal/proto"
)

// roundTrip writes a single framed message to a bytes.Buffer then reads it
// back. It returns the recovered type and payload.
func roundTrip(t *testing.T, msgType proto.MsgType, payload []byte) (proto.MsgType, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if err := proto.WriteMsg(&buf, msgType, payload); err != nil {
		t.Fatalf("WriteMsg(%s): %v", msgType, err)
	}
	got, gotPayload, err := proto.ReadMsg(&buf)
	if err != nil {
		t.Fatalf("ReadMsg(%s): %v", msgType, err)
	}
	if got != msgType {
		t.Errorf("type: got %s, want %s", got, msgType)
	}
	return got, gotPayload
}

// ---- Framing layer ----------------------------------------------------------

func TestFrameRoundTrip_EmptyPayload(t *testing.T) {
	_, payload := roundTrip(t, proto.MsgIdentifyDone, nil)
	if len(payload) != 0 {
		t.Errorf("expected empty payload, got %d bytes", len(payload))
	}
}

func TestFrameRoundTrip_NonEmptyPayload(t *testing.T) {
	data := []byte("hello, world")
	_, payload := roundTrip(t, proto.MsgStdin, data)
	if !bytes.Equal(payload, data) {
		t.Errorf("payload mismatch: got %q, want %q", payload, data)
	}
}

func TestFrameMultipleMessages(t *testing.T) {
	var buf bytes.Buffer

	msgs := []struct {
		t proto.MsgType
		p []byte
	}{
		{proto.MsgVersion, []byte{0, 1}},
		{proto.MsgStdin, []byte("input bytes")},
		{proto.MsgIdentifyDone, nil},
	}

	for _, m := range msgs {
		if err := proto.WriteMsg(&buf, m.t, m.p); err != nil {
			t.Fatalf("WriteMsg: %v", err)
		}
	}

	for _, want := range msgs {
		got, payload, err := proto.ReadMsg(&buf)
		if err != nil {
			t.Fatalf("ReadMsg: %v", err)
		}
		if got != want.t {
			t.Errorf("type: got %s, want %s", got, want.t)
		}
		if !bytes.Equal(payload, want.p) {
			t.Errorf("payload for %s: got %q, want %q", want.t, payload, want.p)
		}
	}
}

func TestFrameEOFOnEmpty(t *testing.T) {
	var buf bytes.Buffer
	_, _, err := proto.ReadMsg(&buf)
	if err != io.EOF && err != io.ErrUnexpectedEOF {
		t.Errorf("expected EOF-class error, got %v", err)
	}
}

func TestFramePayloadTooLarge(t *testing.T) {
	// Craft a header claiming a 32 MiB payload (> maxPayload of 16 MiB).
	var buf bytes.Buffer
	hdr := []byte{
		0, 0, 0, byte(proto.MsgStdin), // type
		0x02, 0x00, 0x00, 0x00,        // length = 0x02000000 = 33554432 (32 MiB)
	}
	buf.Write(hdr)
	_, _, err := proto.ReadMsg(&buf)
	if err == nil {
		t.Fatal("expected error for oversized payload, got nil")
	}
}

// ---- Identify category ------------------------------------------------------

func TestIdentifyFlagsRoundTrip(t *testing.T) {
	orig := proto.IdentifyFlagsMsg{Flags: 0xDEADBEEF}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgIdentifyFlags, payload)

	var got proto.IdentifyFlagsMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if got.Flags != orig.Flags {
		t.Errorf("Flags: got 0x%X, want 0x%X", got.Flags, orig.Flags)
	}
}

func TestIdentifyTermRoundTrip(t *testing.T) {
	orig := proto.IdentifyTermMsg{Term: "xterm-256color"}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgIdentifyTerm, payload)

	var got proto.IdentifyTermMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if got.Term != orig.Term {
		t.Errorf("Term: got %q, want %q", got.Term, orig.Term)
	}
}

func TestIdentifyTermRoundTrip_Empty(t *testing.T) {
	orig := proto.IdentifyTermMsg{Term: ""}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgIdentifyTerm, payload)

	var got proto.IdentifyTermMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if got.Term != "" {
		t.Errorf("Term: got %q, want empty string", got.Term)
	}
}

func TestIdentifyTerminfoRoundTrip(t *testing.T) {
	orig := proto.IdentifyTerminfoMsg{Data: []byte{0x1b, 0x5b, 0x41, 0x00, 0xFF}}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgIdentifyTerminfo, payload)

	var got proto.IdentifyTerminfoMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Data, orig.Data) {
		t.Errorf("Data mismatch")
	}
}

func TestIdentifyTTYNameRoundTrip(t *testing.T) {
	orig := proto.IdentifyTTYNameMsg{TTYName: "/dev/pts/3"}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgIdentifyTTYName, payload)

	var got proto.IdentifyTTYNameMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if got.TTYName != orig.TTYName {
		t.Errorf("TTYName: got %q, want %q", got.TTYName, orig.TTYName)
	}
}

func TestIdentifyCWDRoundTrip(t *testing.T) {
	orig := proto.IdentifyCWDMsg{CWD: "/home/user/projects"}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgIdentifyCWD, payload)

	var got proto.IdentifyCWDMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if got.CWD != orig.CWD {
		t.Errorf("CWD: got %q, want %q", got.CWD, orig.CWD)
	}
}

func TestIdentifyEnvironRoundTrip(t *testing.T) {
	orig := proto.IdentifyEnvironMsg{
		Pairs: []string{"TERM=xterm-256color", "LANG=en_US.UTF-8", "HOME=/root"},
	}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgIdentifyEnviron, payload)

	var got proto.IdentifyEnvironMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if len(got.Pairs) != len(orig.Pairs) {
		t.Fatalf("Pairs len: got %d, want %d", len(got.Pairs), len(orig.Pairs))
	}
	for i := range orig.Pairs {
		if got.Pairs[i] != orig.Pairs[i] {
			t.Errorf("Pairs[%d]: got %q, want %q", i, got.Pairs[i], orig.Pairs[i])
		}
	}
}

func TestIdentifyEnvironRoundTrip_Empty(t *testing.T) {
	orig := proto.IdentifyEnvironMsg{Pairs: []string{}}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgIdentifyEnviron, payload)

	var got proto.IdentifyEnvironMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if len(got.Pairs) != 0 {
		t.Errorf("expected empty Pairs, got %v", got.Pairs)
	}
}

func TestIdentifyClientPIDRoundTrip(t *testing.T) {
	orig := proto.IdentifyClientPIDMsg{PID: 12345}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgIdentifyClientPID, payload)

	var got proto.IdentifyClientPIDMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if got.PID != orig.PID {
		t.Errorf("PID: got %d, want %d", got.PID, orig.PID)
	}
}

func TestIdentifyClientPIDRoundTrip_Negative(t *testing.T) {
	orig := proto.IdentifyClientPIDMsg{PID: -1}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgIdentifyClientPID, payload)

	var got proto.IdentifyClientPIDMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if got.PID != -1 {
		t.Errorf("PID: got %d, want -1", got.PID)
	}
}

func TestIdentifyFeaturesRoundTrip(t *testing.T) {
	orig := proto.IdentifyFeaturesMsg{Features: 0x0000_00FF}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgIdentifyFeatures, payload)

	var got proto.IdentifyFeaturesMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if got.Features != orig.Features {
		t.Errorf("Features: got 0x%X, want 0x%X", got.Features, orig.Features)
	}
}

func TestIdentifyDoneRoundTrip(t *testing.T) {
	orig := proto.IdentifyDoneMsg{}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgIdentifyDone, payload)

	var got proto.IdentifyDoneMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
}

// ---- Session category -------------------------------------------------------

func TestVersionRoundTrip(t *testing.T) {
	orig := proto.VersionMsg{Version: 3}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgVersion, payload)

	var got proto.VersionMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if got.Version != orig.Version {
		t.Errorf("Version: got %d, want %d", got.Version, orig.Version)
	}
}

func TestCommandRoundTrip(t *testing.T) {
	orig := proto.CommandMsg{Argv: []string{"new-window", "-t", "mywin"}}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgCommand, payload)

	var got proto.CommandMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if len(got.Argv) != len(orig.Argv) {
		t.Fatalf("Argv len: got %d, want %d", len(got.Argv), len(orig.Argv))
	}
	for i := range orig.Argv {
		if got.Argv[i] != orig.Argv[i] {
			t.Errorf("Argv[%d]: got %q, want %q", i, got.Argv[i], orig.Argv[i])
		}
	}
}

func TestCommandRoundTrip_NoArgs(t *testing.T) {
	orig := proto.CommandMsg{Argv: []string{"list-windows"}}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgCommand, payload)

	var got proto.CommandMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if len(got.Argv) != 1 || got.Argv[0] != "list-windows" {
		t.Errorf("Argv: got %v, want [list-windows]", got.Argv)
	}
}

func TestResizeRoundTrip(t *testing.T) {
	orig := proto.ResizeMsg{Width: 220, Height: 50}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgResize, payload)

	var got proto.ResizeMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if got.Width != orig.Width || got.Height != orig.Height {
		t.Errorf("Resize: got %dx%d, want %dx%d", got.Width, got.Height, orig.Width, orig.Height)
	}
}

func TestDetachRoundTrip(t *testing.T) {
	orig := proto.DetachMsg{}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgDetach, payload)

	var got proto.DetachMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
}

func TestExitRoundTrip(t *testing.T) {
	orig := proto.ExitMsg{Code: 42}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgExit, payload)

	var got proto.ExitMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if got.Code != orig.Code {
		t.Errorf("Code: got %d, want %d", got.Code, orig.Code)
	}
}

func TestExitedRoundTrip(t *testing.T) {
	orig := proto.ExitedMsg{Code: 1}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgExited, payload)

	var got proto.ExitedMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if got.Code != orig.Code {
		t.Errorf("Code: got %d, want %d", got.Code, orig.Code)
	}
}

func TestShutdownRoundTrip(t *testing.T) {
	orig := proto.ShutdownMsg{}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgShutdown, payload)

	var got proto.ShutdownMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
}

// ---- Data category ----------------------------------------------------------

func TestStdinRoundTrip(t *testing.T) {
	orig := proto.StdinMsg{Data: []byte("ls -la\n")}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgStdin, payload)

	var got proto.StdinMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Data, orig.Data) {
		t.Errorf("Data: got %q, want %q", got.Data, orig.Data)
	}
}

func TestStdinRoundTrip_Binary(t *testing.T) {
	// Escape sequence: ESC [ A (cursor up)
	orig := proto.StdinMsg{Data: []byte{0x1b, 0x5b, 0x41}}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgStdin, payload)

	var got proto.StdinMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Data, orig.Data) {
		t.Errorf("Data mismatch: got %v, want %v", got.Data, orig.Data)
	}
}

func TestStdoutRoundTrip(t *testing.T) {
	orig := proto.StdoutMsg{Data: []byte("\x1b[H\x1b[2J")}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgStdout, payload)

	var got proto.StdoutMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Data, orig.Data) {
		t.Errorf("Data: got %q, want %q", got.Data, orig.Data)
	}
}

func TestStderrRoundTrip(t *testing.T) {
	orig := proto.StderrMsg{Data: []byte("error: something went wrong\n")}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgStderr, payload)

	var got proto.StderrMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Data, orig.Data) {
		t.Errorf("Data: got %q, want %q", got.Data, orig.Data)
	}
}

// ---- File RPC category ------------------------------------------------------

func TestReadOpenRoundTrip(t *testing.T) {
	orig := proto.ReadOpenMsg{Path: "/tmp/buffer.txt"}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgReadOpen, payload)

	var got proto.ReadOpenMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if got.Path != orig.Path {
		t.Errorf("Path: got %q, want %q", got.Path, orig.Path)
	}
}

func TestFileReadMsgRoundTrip(t *testing.T) {
	orig := proto.FileReadMsg{Data: []byte("file contents here\n")}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgRead, payload)

	var got proto.FileReadMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Data, orig.Data) {
		t.Errorf("Data: got %q, want %q", got.Data, orig.Data)
	}
}

func TestReadDoneRoundTrip(t *testing.T) {
	orig := proto.ReadDoneMsg{}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgReadDone, payload)

	var got proto.ReadDoneMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
}

func TestWriteOpenRoundTrip(t *testing.T) {
	orig := proto.WriteOpenMsg{Path: "/home/user/output.txt"}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgWriteOpen, payload)

	var got proto.WriteOpenMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if got.Path != orig.Path {
		t.Errorf("Path: got %q, want %q", got.Path, orig.Path)
	}
}

func TestFileWriteMsgRoundTrip(t *testing.T) {
	orig := proto.FileWriteMsg{Data: []byte("data to write\n")}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgWrite, payload)

	var got proto.FileWriteMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Data, orig.Data) {
		t.Errorf("Data: got %q, want %q", got.Data, orig.Data)
	}
}

func TestWriteReadyRoundTrip(t *testing.T) {
	orig := proto.WriteReadyMsg{}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgWriteReady, payload)

	var got proto.WriteReadyMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
}

func TestWriteCloseRoundTrip(t *testing.T) {
	orig := proto.WriteCloseMsg{}
	payload := orig.Encode()

	_, raw := roundTrip(t, proto.MsgWriteClose, payload)

	var got proto.WriteCloseMsg
	if err := got.Decode(raw); err != nil {
		t.Fatal(err)
	}
}

// ---- Full identify sequence -------------------------------------------------

// TestIdentifySequence exercises the full client-attach handshake in order,
// using a bytes.Buffer as the transport (no network required).
func TestIdentifySequence(t *testing.T) {
	var buf bytes.Buffer

	// Write the sequence as a client would.
	msgs := []struct {
		t       proto.MsgType
		payload []byte
	}{
		{proto.MsgVersion, proto.VersionMsg{Version: 1}.Encode()},
		{proto.MsgIdentifyFlags, proto.IdentifyFlagsMsg{Flags: 0x01}.Encode()},
		{proto.MsgIdentifyTerm, proto.IdentifyTermMsg{Term: "screen-256color"}.Encode()},
		{proto.MsgIdentifyTerminfo, proto.IdentifyTerminfoMsg{Data: []byte{0x1b}}.Encode()},
		{proto.MsgIdentifyTTYName, proto.IdentifyTTYNameMsg{TTYName: "/dev/pts/0"}.Encode()},
		{proto.MsgIdentifyCWD, proto.IdentifyCWDMsg{CWD: "/workspace"}.Encode()},
		{proto.MsgIdentifyEnviron, proto.IdentifyEnvironMsg{Pairs: []string{"TERM=screen-256color"}}.Encode()},
		{proto.MsgIdentifyClientPID, proto.IdentifyClientPIDMsg{PID: 9999}.Encode()},
		{proto.MsgIdentifyFeatures, proto.IdentifyFeaturesMsg{Features: 0x03}.Encode()},
		{proto.MsgIdentifyDone, proto.IdentifyDoneMsg{}.Encode()},
	}

	for _, m := range msgs {
		if err := proto.WriteMsg(&buf, m.t, m.payload); err != nil {
			t.Fatalf("WriteMsg(%s): %v", m.t, err)
		}
	}

	// Read back and decode as a server would.
	for _, want := range msgs {
		gotType, gotPayload, err := proto.ReadMsg(&buf)
		if err != nil {
			t.Fatalf("ReadMsg: %v", err)
		}
		if gotType != want.t {
			t.Errorf("type: got %s, want %s", gotType, want.t)
		}
		if !bytes.Equal(gotPayload, want.payload) {
			t.Errorf("payload for %s: got %v, want %v", want.t, gotPayload, want.payload)
		}
	}
}

// TestFileRPCSequence exercises the full read file-RPC exchange using a
// bytes.Buffer as the transport.
func TestFileRPCSequence(t *testing.T) {
	var buf bytes.Buffer

	chunk1 := []byte("first chunk\n")
	chunk2 := []byte("second chunk\n")

	// Server side: ask client to read /tmp/test.txt
	messages := []struct {
		t       proto.MsgType
		payload []byte
	}{
		{proto.MsgReadOpen, proto.ReadOpenMsg{Path: "/tmp/test.txt"}.Encode()},
		{proto.MsgRead, proto.FileReadMsg{Data: chunk1}.Encode()},
		{proto.MsgRead, proto.FileReadMsg{Data: chunk2}.Encode()},
		{proto.MsgReadDone, proto.ReadDoneMsg{}.Encode()},
	}

	for _, m := range messages {
		if err := proto.WriteMsg(&buf, m.t, m.payload); err != nil {
			t.Fatalf("WriteMsg(%s): %v", m.t, err)
		}
	}

	// Read back READ_OPEN
	msgType, payload, err := proto.ReadMsg(&buf)
	if err != nil || msgType != proto.MsgReadOpen {
		t.Fatalf("expected READ_OPEN, got %s (err=%v)", msgType, err)
	}
	var openMsg proto.ReadOpenMsg
	if err := openMsg.Decode(payload); err != nil {
		t.Fatal(err)
	}
	if openMsg.Path != "/tmp/test.txt" {
		t.Errorf("Path: got %q", openMsg.Path)
	}

	// Read back two READ chunks
	var allData []byte
	for i := 0; i < 2; i++ {
		msgType, payload, err = proto.ReadMsg(&buf)
		if err != nil || msgType != proto.MsgRead {
			t.Fatalf("expected READ, got %s (err=%v)", msgType, err)
		}
		var readMsg proto.FileReadMsg
		if err := readMsg.Decode(payload); err != nil {
			t.Fatal(err)
		}
		allData = append(allData, readMsg.Data...)
	}

	want := append(chunk1, chunk2...)
	if !bytes.Equal(allData, want) {
		t.Errorf("read data: got %q, want %q", allData, want)
	}

	// Read back READ_DONE
	msgType, _, err = proto.ReadMsg(&buf)
	if err != nil || msgType != proto.MsgReadDone {
		t.Fatalf("expected READ_DONE, got %s (err=%v)", msgType, err)
	}
}
