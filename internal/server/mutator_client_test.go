package server

import (
	"bytes"
	"net"
	"testing"

	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/session"
)

// newTestMutatorWithConns creates a serverMutator with injectable getConn/markDirty.
func newTestMutatorWithConns(
	getConn func(session.ClientID) (*clientConn, bool),
	markDirty func(*clientConn),
) (*serverMutator, chan struct{}) {
	state := session.NewServer()
	done := make(chan struct{})
	m := &serverMutator{
		state:     state,
		shutdown:  func() { close(done) },
		getConn:   getConn,
		markDirty: markDirty,
	}
	return m, done
}

// addTestClient registers a bare *session.Client in the mutator's state.
func addTestClient(m *serverMutator, clientID string) *session.Client {
	c := session.NewClient(session.ClientID(clientID))
	m.state.Clients[c.ID] = c
	return c
}

func TestAttachClient(t *testing.T) {
	m, _ := newTestMutator()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := addTestClient(m, "c1")

	if err := m.AttachClient("c1", sv.ID); err != nil {
		t.Fatalf("AttachClient: %v", err)
	}
	if c.Session == nil {
		t.Fatal("client.Session is nil after AttachClient")
	}
	if string(c.Session.ID) != sv.ID {
		t.Errorf("client.Session.ID = %q, want %q", c.Session.ID, sv.ID)
	}
}

func TestAttachClientNotFound(t *testing.T) {
	m, _ := newTestMutator()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := m.AttachClient("no-such-client", sv.ID); err == nil {
		t.Error("expected error for unknown client, got nil")
	}
}

func TestDetachClient(t *testing.T) {
	m, _ := newTestMutator()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := addTestClient(m, "c1")
	if err := m.AttachClient("c1", sv.ID); err != nil {
		t.Fatalf("AttachClient: %v", err)
	}

	if err := m.DetachClient("c1"); err != nil {
		t.Fatalf("DetachClient: %v", err)
	}
	// After DetachClient the client is removed from the Clients map.
	if _, ok := m.state.Clients[c.ID]; ok {
		t.Error("client still present in Clients map after DetachClient")
	}
}

func TestDetachClientNotFound(t *testing.T) {
	m, _ := newTestMutator()

	if err := m.DetachClient("no-such-client"); err == nil {
		t.Error("expected error for unknown client, got nil")
	}
}

func TestSwitchClient(t *testing.T) {
	m, _ := newTestMutator()

	sv1, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession s1: %v", err)
	}
	sv2, err := m.NewSession("s2")
	if err != nil {
		t.Fatalf("NewSession s2: %v", err)
	}
	c := addTestClient(m, "c1")
	if err := m.AttachClient("c1", sv1.ID); err != nil {
		t.Fatalf("AttachClient: %v", err)
	}

	if err := m.SwitchClient("c1", sv2.ID); err != nil {
		t.Fatalf("SwitchClient: %v", err)
	}
	if c.Session == nil {
		t.Fatal("client.Session is nil after SwitchClient")
	}
	if string(c.Session.ID) != sv2.ID {
		t.Errorf("client.Session.ID = %q, want %q", c.Session.ID, sv2.ID)
	}
}

func TestSwitchClientNotFound(t *testing.T) {
	m, _ := newTestMutator()

	sv, err := m.NewSession("s1")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := m.SwitchClient("no-such-client", sv.ID); err == nil {
		t.Error("expected error for unknown client, got nil")
	}
}

func TestSwitchClientSessionNotFound(t *testing.T) {
	m, _ := newTestMutator()

	addTestClient(m, "c1")
	if err := m.SwitchClient("c1", "no-such-session"); err == nil {
		t.Error("expected error for unknown session, got nil")
	}
}

func TestDisplayMessage(t *testing.T) {
	// Build a fake net.Conn pair: server side writes, client side reads.
	serverConn, clientConn2 := net.Pipe()
	defer serverConn.Close()
	defer clientConn2.Close()

	cc := &clientConn{
		id:      session.ClientID("c1"),
		netConn: serverConn,
		dirty:   make(chan struct{}, 1),
	}

	var dirtyCalled bool
	m, _ := newTestMutatorWithConns(
		func(id session.ClientID) (*clientConn, bool) {
			if id == "c1" {
				return cc, true
			}
			return nil, false
		},
		func(c *clientConn) { dirtyCalled = true },
	)

	// Run DisplayMessage in a goroutine since net.Pipe is synchronous.
	errCh := make(chan error, 1)
	go func() {
		errCh <- m.DisplayMessage("c1", "hello world")
	}()

	// Read the frame from the client side.
	msgType, payload, err := proto.ReadMsg(clientConn2)
	if err != nil {
		t.Fatalf("ReadMsg: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("DisplayMessage: %v", err)
	}

	if msgType != proto.MsgStdout {
		t.Errorf("msgType = %v, want MsgStdout", msgType)
	}
	var out proto.StdoutMsg
	if err := out.Decode(payload); err != nil {
		t.Fatalf("Decode StdoutMsg: %v", err)
	}
	want := []byte("hello world\r\n")
	if !bytes.Equal(out.Data, want) {
		t.Errorf("data = %q, want %q", out.Data, want)
	}
	if !dirtyCalled {
		t.Error("markDirty was not called")
	}
}

func TestDisplayMessageClientNotFound(t *testing.T) {
	m, _ := newTestMutatorWithConns(
		func(id session.ClientID) (*clientConn, bool) { return nil, false },
		func(c *clientConn) {},
	)
	if err := m.DisplayMessage("no-such-client", "hi"); err == nil {
		t.Error("expected error for unknown client, got nil")
	}
}
