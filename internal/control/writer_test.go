package control

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

// stubSource is an in-memory EventSource for testing. Calling push delivers
// an event synchronously to all registered handlers.
type stubSource struct {
	handlers []func(Event)
}

func (s *stubSource) Subscribe(h func(Event)) func() {
	s.handlers = append(s.handlers, h)
	idx := len(s.handlers) - 1
	return func() {
		// Replace the handler with a no-op to preserve slice indices.
		s.handlers[idx] = nil
	}
}

// push delivers e to all active handlers.
func (s *stubSource) push(e Event) {
	for _, h := range s.handlers {
		if h != nil {
			h(e)
		}
	}
}

// TestOutputEvent verifies that OutputEvent is serialised as
// "%output %<pane> <base64>\n".
func TestOutputEvent(t *testing.T) {
	var buf bytes.Buffer
	src := &stubSource{}
	w := NewWriter(&buf, src)
	defer w.Close()

	data := []byte("hello world")
	src.push(OutputEvent{PaneID: "3", Data: data})

	want := "%output %3 " + base64.StdEncoding.EncodeToString(data) + "\n"
	if got := buf.String(); got != want {
		t.Errorf("OutputEvent:\ngot  %q\nwant %q", got, want)
	}
}

// TestSessionChangedEvent verifies %session-changed serialisation.
func TestSessionChangedEvent(t *testing.T) {
	var buf bytes.Buffer
	src := &stubSource{}
	w := NewWriter(&buf, src)
	defer w.Close()

	src.push(SessionChangedEvent{SessionID: "main", Name: "work"})

	want := "%session-changed main work\n"
	if got := buf.String(); got != want {
		t.Errorf("SessionChangedEvent:\ngot  %q\nwant %q", got, want)
	}
}

// TestWindowAddEvent verifies %window-add serialisation.
func TestWindowAddEvent(t *testing.T) {
	var buf bytes.Buffer
	src := &stubSource{}
	w := NewWriter(&buf, src)
	defer w.Close()

	src.push(WindowAddEvent{WindowID: "42"})

	want := "%window-add 42\n"
	if got := buf.String(); got != want {
		t.Errorf("WindowAddEvent:\ngot  %q\nwant %q", got, want)
	}
}

// TestWindowCloseEvent verifies %window-close serialisation.
func TestWindowCloseEvent(t *testing.T) {
	var buf bytes.Buffer
	src := &stubSource{}
	w := NewWriter(&buf, src)
	defer w.Close()

	src.push(WindowCloseEvent{WindowID: "7"})

	want := "%window-close 7\n"
	if got := buf.String(); got != want {
		t.Errorf("WindowCloseEvent:\ngot  %q\nwant %q", got, want)
	}
}

// TestWindowPaneChangedEvent verifies %window-pane-changed serialisation.
func TestWindowPaneChangedEvent(t *testing.T) {
	var buf bytes.Buffer
	src := &stubSource{}
	w := NewWriter(&buf, src)
	defer w.Close()

	src.push(WindowPaneChangedEvent{WindowID: "2", PaneID: "5"})

	want := "%window-pane-changed 2 5\n"
	if got := buf.String(); got != want {
		t.Errorf("WindowPaneChangedEvent:\ngot  %q\nwant %q", got, want)
	}
}

// TestLayoutChangeEvent verifies %layout-change serialisation.
func TestLayoutChangeEvent(t *testing.T) {
	var buf bytes.Buffer
	src := &stubSource{}
	w := NewWriter(&buf, src)
	defer w.Close()

	src.push(LayoutChangeEvent{WindowID: "1", Layout: "5fda,220x50,0,0"})

	want := "%layout-change 1 5fda,220x50,0,0\n"
	if got := buf.String(); got != want {
		t.Errorf("LayoutChangeEvent:\ngot  %q\nwant %q", got, want)
	}
}

// TestExitEvent verifies %exit serialisation.
func TestExitEvent(t *testing.T) {
	var buf bytes.Buffer
	src := &stubSource{}
	w := NewWriter(&buf, src)
	defer w.Close()

	src.push(ExitEvent{Reason: "server exiting"})

	want := "%exit server exiting\n"
	if got := buf.String(); got != want {
		t.Errorf("ExitEvent:\ngot  %q\nwant %q", got, want)
	}
}

// TestBeginEndEvents verifies %begin/%end block serialisation.
func TestBeginEndEvents(t *testing.T) {
	var buf bytes.Buffer
	src := &stubSource{}
	w := NewWriter(&buf, src)
	defer w.Close()

	src.push(BeginEvent{Time: 1700000000, Number: 1, Flags: 0})
	src.push(EndEvent{Time: 1700000001, Number: 1, Flags: 0})

	got := buf.String()
	if !strings.Contains(got, "%begin 1700000000 1 0\n") {
		t.Errorf("BeginEvent not found in output: %q", got)
	}
	if !strings.Contains(got, "%end 1700000001 1 0\n") {
		t.Errorf("EndEvent not found in output: %q", got)
	}
}

// TestSubscriptionLifecycle verifies that after Close, events are no longer
// delivered to the Writer's underlying io.Writer.
func TestSubscriptionLifecycle(t *testing.T) {
	var buf bytes.Buffer
	src := &stubSource{}
	w := NewWriter(&buf, src)

	// Deliver one event before closing.
	src.push(WindowAddEvent{WindowID: "1"})
	before := buf.Len()
	if before == 0 {
		t.Fatal("expected output before Close, got none")
	}

	w.Close()
	buf.Reset()

	// Deliver an event after closing — nothing should be written.
	src.push(WindowAddEvent{WindowID: "2"})
	if buf.Len() != 0 {
		t.Errorf("expected no output after Close, got %q", buf.String())
	}
}

// TestMultipleSubscribers verifies that multiple Writers can subscribe
// to the same EventSource independently.
func TestMultipleSubscribers(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	src := &stubSource{}

	w1 := NewWriter(&buf1, src)
	w2 := NewWriter(&buf2, src)
	defer w1.Close()
	defer w2.Close()

	src.push(WindowAddEvent{WindowID: "9"})

	want := "%window-add 9\n"
	if got := buf1.String(); got != want {
		t.Errorf("writer1: got %q, want %q", got, want)
	}
	if got := buf2.String(); got != want {
		t.Errorf("writer2: got %q, want %q", got, want)
	}
}
