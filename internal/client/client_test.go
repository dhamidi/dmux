package client_test

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/dhamidi/dmux/internal/client"
	"github.com/dhamidi/dmux/internal/proto"
)

// noopSize is a SizeFunc stub that reports a 24-row × 80-column terminal.
func noopSize() (int, int, error) { return 24, 80, nil }

// readHandshake reads and validates the VERSION + IDENTIFY_* + RESIZE sequence
// from r. It asserts:
//   - VERSION message with version == 1
//   - IDENTIFY_FLAGS, IDENTIFY_TERM, IDENTIFY_CWD, IDENTIFY_ENVIRON,
//     IDENTIFY_CLIENTPID, IDENTIFY_FEATURES, IDENTIFY_DONE in that order
//   - RESIZE message (sent after IDENTIFY_DONE)
func readHandshake(r io.Reader) error {
	// VERSION
	mt, payload, err := proto.ReadMsg(r)
	if err != nil {
		return fmt.Errorf("reading VERSION: %w", err)
	}
	if mt != proto.MsgVersion {
		return fmt.Errorf("want VERSION, got %s", mt)
	}
	var vm proto.VersionMsg
	if err := vm.Decode(payload); err != nil {
		return err
	}
	if vm.Version != 1 {
		return fmt.Errorf("want protocol version 1, got %d", vm.Version)
	}

	// IDENTIFY sequence (must arrive in this exact order)
	wantSeq := []proto.MsgType{
		proto.MsgIdentifyFlags,
		proto.MsgIdentifyTerm,
		proto.MsgIdentifyCWD,
		proto.MsgIdentifyEnviron,
		proto.MsgIdentifyClientPID,
		proto.MsgIdentifyFeatures,
		proto.MsgIdentifyDone,
	}
	for _, want := range wantSeq {
		got, _, err := proto.ReadMsg(r)
		if err != nil {
			return fmt.Errorf("reading %s: %w", want, err)
		}
		if got != want {
			return fmt.Errorf("want %s, got %s", want, got)
		}
	}

	// RESIZE is sent immediately after IDENTIFY_DONE.
	got, _, err := proto.ReadMsg(r)
	if err != nil {
		return fmt.Errorf("reading RESIZE: %w", err)
	}
	if got != proto.MsgResize {
		return fmt.Errorf("want RESIZE, got %s", got)
	}

	return nil
}

// TestHandshake verifies that Run sends a correct VERSION and full IDENTIFY
// sequence when it connects. The fake server closes the connection after
// validating the handshake; Run must return 0 (clean exit).
func TestHandshake(t *testing.T) {
	serverConn, clientConn := net.Pipe()

	errc := make(chan error, 1)
	go func() {
		defer serverConn.Close()
		errc <- readHandshake(serverConn)
	}()

	code := client.Run(client.Config{
		Conn:     clientConn,
		In:       bytes.NewReader(nil),
		Out:      io.Discard,
		Size:     noopSize,
		ReadOnly: true,
	})

	if code != 0 {
		t.Errorf("Run() = %d, want 0", code)
	}
	if err := <-errc; err != nil {
		t.Error(err)
	}
}

// TestDataForwarding verifies bidirectional data forwarding:
//   - STDOUT sent by the server appears in cfg.Out.
//   - Bytes from cfg.In are forwarded as STDIN messages to the server.
//
// The fake server also sends EXIT 42, so Run must return 42.
func TestDataForwarding(t *testing.T) {
	serverConn, clientConn := net.Pipe()

	const serverOutput = "hello from server"
	const clientInput = "world"

	errc := make(chan error, 1)
	go func() {
		defer serverConn.Close()

		// Consume handshake before sending data.
		if err := readHandshake(serverConn); err != nil {
			errc <- err
			return
		}

		// Send STDOUT to the client.
		sm := proto.StdoutMsg{Data: []byte(serverOutput)}
		if err := proto.WriteMsg(serverConn, proto.MsgStdout, sm.Encode()); err != nil {
			errc <- fmt.Errorf("writing STDOUT: %w", err)
			return
		}

		// Read the STDIN message forwarded by the client's pumpStdin goroutine.
		mt, payload, err := proto.ReadMsg(serverConn)
		if err != nil {
			errc <- fmt.Errorf("reading STDIN: %w", err)
			return
		}
		if mt != proto.MsgStdin {
			errc <- fmt.Errorf("want STDIN, got %s", mt)
			return
		}
		var in proto.StdinMsg
		if err := in.Decode(payload); err != nil {
			errc <- err
			return
		}
		if string(in.Data) != clientInput {
			errc <- fmt.Errorf("STDIN data = %q, want %q", in.Data, clientInput)
			return
		}

		// Tell the client to exit with a non-zero code.
		em := proto.ExitMsg{Code: 42}
		if err := proto.WriteMsg(serverConn, proto.MsgExit, em.Encode()); err != nil {
			errc <- fmt.Errorf("writing EXIT: %w", err)
			return
		}

		errc <- nil
	}()

	out := &bytes.Buffer{}
	code := client.Run(client.Config{
		Conn: clientConn,
		In:   bytes.NewReader([]byte(clientInput)),
		Out:  out,
		Size: noopSize,
	})

	if code != 42 {
		t.Errorf("Run() = %d, want 42", code)
	}
	wantOutput := "\x1b[?1049h" + serverOutput + "\x1b[?1049l"
	if got := out.String(); got != wantOutput {
		t.Errorf("output = %q, want %q", got, wantOutput)
	}
	if err := <-errc; err != nil {
		t.Error(err)
	}
}

// TestCleanExitOnServerClose verifies that Run returns 0 when the server
// closes the connection without sending an EXIT message.
func TestCleanExitOnServerClose(t *testing.T) {
	serverConn, clientConn := net.Pipe()

	errc := make(chan error, 1)
	go func() {
		defer serverConn.Close()
		// Read the handshake and close — no EXIT message.
		errc <- readHandshake(serverConn)
	}()

	code := client.Run(client.Config{
		Conn:     clientConn,
		In:       bytes.NewReader(nil),
		Out:      io.Discard,
		Size:     noopSize,
		ReadOnly: true,
	})

	if code != 0 {
		t.Errorf("Run() = %d, want 0 on clean server close", code)
	}
	if err := <-errc; err != nil {
		t.Error(err)
	}
}
