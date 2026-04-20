package control_test

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/dhamidi/dmux/internal/command"
	_ "github.com/dhamidi/dmux/internal/command/builtin"
	"github.com/dhamidi/dmux/internal/control"
)

// stubServer is a minimal command.Server for tests.
type stubServer struct{}

func (s *stubServer) GetSession(id string) (command.SessionView, bool) { return command.SessionView{}, false }
func (s *stubServer) GetSessionByName(name string) (command.SessionView, bool) {
	return command.SessionView{}, false
}
func (s *stubServer) ListSessions() []command.SessionView { return nil }
func (s *stubServer) GetClient(id string) (command.ClientView, bool) {
	return command.ClientView{}, false
}
func (s *stubServer) ListClients() []command.ClientView { return nil }

// TestHandleCommandWrapsOutput verifies that a valid command produces a
// %begin/%end block in the output.
func TestHandleCommandWrapsOutput(t *testing.T) {
	var buf bytes.Buffer
	cs := control.NewControlSession(&buf, &stubServer{}, nil, nil, command.ClientView{})
	defer cs.Close()

	cs.HandleCommand([]string{"list-sessions"})

	got := buf.String()
	if !strings.Contains(got, "%begin ") {
		t.Errorf("expected %%begin in output, got: %q", got)
	}
	if !strings.Contains(got, "%end ") {
		t.Errorf("expected %%end in output, got: %q", got)
	}
}

// TestHandleCommandError verifies that a failing command produces a
// %begin/%error block instead of %begin/%end.
func TestHandleCommandError(t *testing.T) {
	var buf bytes.Buffer
	cs := control.NewControlSession(&buf, &stubServer{}, nil, nil, command.ClientView{})
	defer cs.Close()

	// "no-such-command" is not registered; dispatch returns an error.
	cs.HandleCommand([]string{"no-such-command"})

	got := buf.String()
	if !strings.Contains(got, "%begin ") {
		t.Errorf("expected %%begin in output, got: %q", got)
	}
	if !strings.Contains(got, "%error ") {
		t.Errorf("expected %%error in output, got: %q", got)
	}
	if strings.Contains(got, "%end ") {
		t.Errorf("unexpected %%end in error output, got: %q", got)
	}
}

// TestWindowAddEventEmission verifies that publishing a WindowAddEvent through
// the ControlSession's bus produces the correct %window-add line.
func TestWindowAddEventEmission(t *testing.T) {
	var buf bytes.Buffer
	cs := control.NewControlSession(&buf, &stubServer{}, nil, nil, command.ClientView{})
	defer cs.Close()

	cs.Bus().Publish(control.WindowAddEvent{WindowID: "@5"})

	want := "%window-add @5\n"
	if got := buf.String(); got != want {
		t.Errorf("WindowAddEvent:\ngot  %q\nwant %q", got, want)
	}
}

// TestOutputEventBase64 verifies that pane output is emitted as a
// base64-encoded %output line.
func TestOutputEventBase64(t *testing.T) {
	var buf bytes.Buffer
	cs := control.NewControlSession(&buf, &stubServer{}, nil, nil, command.ClientView{})
	defer cs.Close()

	data := []byte("hello control mode")
	cs.Bus().Publish(control.OutputEvent{PaneID: "2", Data: data})

	want := "%output %2 " + base64.StdEncoding.EncodeToString(data) + "\n"
	if got := buf.String(); got != want {
		t.Errorf("OutputEvent:\ngot  %q\nwant %q", got, want)
	}
}

// TestHandleCommandSequenceNumbers verifies that each command gets a unique,
// incrementing sequence number in the %begin/%end block.
func TestHandleCommandSequenceNumbers(t *testing.T) {
	var buf bytes.Buffer
	cs := control.NewControlSession(&buf, &stubServer{}, nil, nil, command.ClientView{})
	defer cs.Close()

	cs.HandleCommand([]string{"list-sessions"})
	cs.HandleCommand([]string{"list-sessions"})

	got := buf.String()
	// First command should have seq 0, second seq 1.
	if !strings.Contains(got, " 0 0\n") {
		t.Errorf("expected seq 0 in output, got: %q", got)
	}
	if !strings.Contains(got, " 1 0\n") {
		t.Errorf("expected seq 1 in output, got: %q", got)
	}
}

// TestResizeClient verifies that ResizeClient updates Width and Height.
func TestResizeClient(t *testing.T) {
	var buf bytes.Buffer
	cs := control.NewControlSession(&buf, &stubServer{}, nil, nil, command.ClientView{})
	defer cs.Close()

	cs.ResizeClient(220, 50)
	if cs.Width != 220 || cs.Height != 50 {
		t.Errorf("ResizeClient: got %dx%d, want 220x50", cs.Width, cs.Height)
	}
}
