package server

import (
	"bytes"
	"image"
	"net"
	"testing"

	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/session"
)

// ─── SetClientFeatures ────────────────────────────────────────────────────────

func TestSetClientFeatures_ParsesBitmask(t *testing.T) {
	m, _ := newTestMutator()
	c := addTestClient(m, "c1")

	if err := m.SetClientFeatures("c1", "256,RGB"); err != nil {
		t.Fatalf("SetClientFeatures: %v", err)
	}
	want := session.FeatureColour256 | session.FeatureColour16M
	if c.Features != want {
		t.Errorf("Features = %v, want %v", c.Features, want)
	}
}

func TestSetClientFeatures_AllFlags(t *testing.T) {
	m, _ := newTestMutator()
	c := addTestClient(m, "c1")

	if err := m.SetClientFeatures("c1", "256,RGB,mouse-sgr,overlap"); err != nil {
		t.Fatalf("SetClientFeatures: %v", err)
	}
	want := session.FeatureColour256 | session.FeatureColour16M | session.FeatureMouseSGR | session.FeatureOverlap
	if c.Features != want {
		t.Errorf("Features = %v, want %v", c.Features, want)
	}
}

func TestSetClientFeatures_UnknownFlagsIgnored(t *testing.T) {
	m, _ := newTestMutator()
	c := addTestClient(m, "c1")

	if err := m.SetClientFeatures("c1", "256,unknown-flag"); err != nil {
		t.Fatalf("SetClientFeatures: %v", err)
	}
	if c.Features != session.FeatureColour256 {
		t.Errorf("Features = %v, want FeatureColour256 only", c.Features)
	}
}

func TestSetClientFeatures_ClientNotFound(t *testing.T) {
	m, _ := newTestMutator()
	if err := m.SetClientFeatures("no-such-client", "256"); err == nil {
		t.Error("expected error for unknown client, got nil")
	}
}

func TestSetClientFeatures_EmptyStringClearsFlags(t *testing.T) {
	m, _ := newTestMutator()
	c := addTestClient(m, "c1")
	c.Features = session.FeatureColour256

	if err := m.SetClientFeatures("c1", ""); err != nil {
		t.Fatalf("SetClientFeatures: %v", err)
	}
	if c.Features != 0 {
		t.Errorf("Features = %v, want 0 after empty string", c.Features)
	}
}

// ─── RequestClientClipboard ───────────────────────────────────────────────────

func TestRequestClientClipboard_SendsOSC52Query(t *testing.T) {
	serverConn, clientSide := net.Pipe()
	defer serverConn.Close()
	defer clientSide.Close()

	cc := &clientConn{
		id:      session.ClientID("c1"),
		netConn: serverConn,
		dirty:   make(chan struct{}, 1),
	}

	m, _ := newTestMutatorWithConns(
		func(id session.ClientID) (*clientConn, bool) {
			if id == "c1" {
				return cc, true
			}
			return nil, false
		},
		func(c *clientConn) {},
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- m.RequestClientClipboard("c1")
	}()

	msgType, payload, err := proto.ReadMsg(clientSide)
	if err != nil {
		t.Fatalf("ReadMsg: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("RequestClientClipboard: %v", err)
	}

	if msgType != proto.MsgStdout {
		t.Errorf("msgType = %v, want MsgStdout", msgType)
	}
	var out proto.StdoutMsg
	if err := out.Decode(payload); err != nil {
		t.Fatalf("Decode StdoutMsg: %v", err)
	}
	want := []byte("\033]52;c;?\033\\")
	if !bytes.Equal(out.Data, want) {
		t.Errorf("OSC 52 query = %q, want %q", out.Data, want)
	}
}

func TestRequestClientClipboard_ClientNotConnected(t *testing.T) {
	m, _ := newTestMutatorWithConns(
		func(id session.ClientID) (*clientConn, bool) { return nil, false },
		func(c *clientConn) {},
	)
	if err := m.RequestClientClipboard("no-such-client"); err == nil {
		t.Error("expected error for unconnected client, got nil")
	}
}

// ─── AddClientSubscription ────────────────────────────────────────────────────

func TestAddClientSubscription_RegistersEntry(t *testing.T) {
	m, _ := newTestMutator()
	c := addTestClient(m, "c1")

	if err := m.AddClientSubscription("c1", "myalert", "bell", "%{window_name}: bell"); err != nil {
		t.Fatalf("AddClientSubscription: %v", err)
	}
	if c.Subscriptions == nil {
		t.Fatal("Subscriptions map is nil after AddClientSubscription")
	}
	sub, ok := c.Subscriptions["myalert"]
	if !ok {
		t.Fatal("subscription 'myalert' not found")
	}
	if sub.Notify != "bell" {
		t.Errorf("sub.Notify = %q, want %q", sub.Notify, "bell")
	}
	if sub.Format != "%{window_name}: bell" {
		t.Errorf("sub.Format = %q, want %q", sub.Format, "%{window_name}: bell")
	}
}

func TestAddClientSubscription_MultipleSubscriptions(t *testing.T) {
	m, _ := newTestMutator()
	c := addTestClient(m, "c1")

	if err := m.AddClientSubscription("c1", "a1", "bell", "bell"); err != nil {
		t.Fatalf("AddClientSubscription a1: %v", err)
	}
	if err := m.AddClientSubscription("c1", "a2", "window-renamed", "renamed"); err != nil {
		t.Fatalf("AddClientSubscription a2: %v", err)
	}
	if len(c.Subscriptions) != 2 {
		t.Errorf("len(Subscriptions) = %d, want 2", len(c.Subscriptions))
	}
}

func TestAddClientSubscription_OverwritesExisting(t *testing.T) {
	m, _ := newTestMutator()
	c := addTestClient(m, "c1")

	m.AddClientSubscription("c1", "a1", "bell", "v1") //nolint:errcheck
	m.AddClientSubscription("c1", "a1", "bell", "v2") //nolint:errcheck

	if c.Subscriptions["a1"].Format != "v2" {
		t.Errorf("expected overwrite; Format = %q, want v2", c.Subscriptions["a1"].Format)
	}
}

func TestAddClientSubscription_ClientNotFound(t *testing.T) {
	m, _ := newTestMutator()
	if err := m.AddClientSubscription("no-such-client", "a", "bell", "fmt"); err == nil {
		t.Error("expected error for unknown client, got nil")
	}
}

// ─── ScrollClientViewport ─────────────────────────────────────────────────────

func TestScrollClientViewport_ShiftsOffset(t *testing.T) {
	serverConn, clientSide := net.Pipe()
	defer serverConn.Close()
	defer clientSide.Close()

	cc := &clientConn{
		id:      session.ClientID("c1"),
		netConn: serverConn,
		dirty:   make(chan struct{}, 1),
	}

	var (
		lastDx, lastDy int
		scrollCalled   bool
	)
	m, _ := newTestMutatorWithConns(
		func(id session.ClientID) (*clientConn, bool) {
			if id == "c1" {
				return cc, true
			}
			return nil, false
		},
		func(c *clientConn) {},
	)
	// Inject a scrollViewportFn that records the call.
	m.scrollViewportFn = func(id session.ClientID, dx, dy int) {
		scrollCalled = true
		lastDx = dx
		lastDy = dy
	}
	// Also register the client in state so the "not found" check passes.
	m.state.Clients[session.ClientID("c1")] = session.NewClient(session.ClientID("c1"))

	if err := m.ScrollClientViewport("c1", 0, 1); err != nil {
		t.Fatalf("ScrollClientViewport: %v", err)
	}
	if !scrollCalled {
		t.Fatal("scrollViewportFn was not called")
	}
	if lastDx != 0 || lastDy != 1 {
		t.Errorf("scrollViewportFn called with (%d, %d), want (0, 1)", lastDx, lastDy)
	}
}

func TestScrollClientViewport_ClientNotFound(t *testing.T) {
	m, _ := newTestMutator()
	if err := m.ScrollClientViewport("no-such-client", 0, 1); err == nil {
		t.Error("expected error for unknown client, got nil")
	}
}

func TestScrollClientViewport_AccumulatesOffset(t *testing.T) {
	// Use the real scrollViewport helper to verify cumulative behaviour.
	accumulated := image.Point{}
	m, _ := newTestMutatorWithConns(
		func(id session.ClientID) (*clientConn, bool) { return nil, false },
		func(c *clientConn) {},
	)
	m.scrollViewportFn = func(_ session.ClientID, dx, dy int) {
		accumulated.X += dx
		accumulated.Y += dy
	}
	m.state.Clients[session.ClientID("c1")] = session.NewClient(session.ClientID("c1"))

	for range 3 {
		if err := m.ScrollClientViewport("c1", 0, 1); err != nil {
			t.Fatalf("ScrollClientViewport: %v", err)
		}
	}
	if err := m.ScrollClientViewport("c1", 0, -1); err != nil {
		t.Fatalf("ScrollClientViewport: %v", err)
	}
	if accumulated.Y != 2 {
		t.Errorf("accumulated Y = %d, want 2", accumulated.Y)
	}
}
