package script

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/xio"
)

// fakeServer is a single-shot fake that listens on an unnamed
// AF_UNIX socketpair through net.Pipe-equivalent: each Dial returns
// one half of a pre-paired connection, and a goroutine on the
// server side reads Identify+CommandList and writes a pre-canned
// CommandResult.
type fakeServer struct {
	mu      sync.Mutex
	calls   [][]string
	results []proto.CommandResult // one per dial, in order
}

// dial implements Dialer using net.Pipe so we never bind a real
// socket. The server-side goroutine reads frames and replies with
// the next pre-canned CommandResult.
func (f *fakeServer) dial(ctx context.Context) (net.Conn, error) {
	clientConn, serverConn := net.Pipe()
	go f.handle(serverConn)
	return clientConn, nil
}

func (f *fakeServer) handle(conn net.Conn) {
	defer conn.Close()

	fr := xio.NewReader(conn)
	fw := xio.NewWriter(conn)

	// Read Identify, then CommandList.
	if _, err := fr.ReadFrame(); err != nil {
		return
	}
	cmdFrame, err := fr.ReadFrame()
	if err != nil {
		return
	}
	cl, ok := cmdFrame.(*proto.CommandList)
	if !ok || len(cl.Commands) == 0 {
		return
	}

	f.mu.Lock()
	f.calls = append(f.calls, cl.Commands[0].Argv)
	idx := len(f.calls) - 1
	var res proto.CommandResult
	if idx < len(f.results) {
		res = f.results[idx]
	} else {
		res = proto.CommandResult{ID: cl.Commands[0].ID, Status: proto.StatusOk}
	}
	f.mu.Unlock()

	res.ID = cl.Commands[0].ID
	_ = fw.WriteFrame(&res)
}

func (f *fakeServer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func TestRunSkipsCommentsAndBlanks(t *testing.T) {
	fs := &fakeServer{}
	script := strings.NewReader(`# header comment

new-session

   # indented comment
client spawn c
`)
	if err := Run(context.Background(), fs.dial, script, RunOptions{Source: "t"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := fs.callCount(), 2; got != want {
		t.Fatalf("RunLine called %d times, want %d (calls=%v)", got, want, fs.calls)
	}
	if fs.calls[0][0] != "new-session" || fs.calls[1][0] != "client" {
		t.Fatalf("unexpected calls: %v", fs.calls)
	}
}

func TestRunStopsOnFirstError(t *testing.T) {
	fs := &fakeServer{
		results: []proto.CommandResult{
			{Status: proto.StatusOk},
			{Status: proto.StatusError, Message: "boom"},
			{Status: proto.StatusOk},
		},
	}
	script := strings.NewReader("first\nsecond\nthird\n")
	err := Run(context.Background(), fs.dial, script, RunOptions{Source: "t"})
	if err == nil {
		t.Fatalf("Run: expected error")
	}
	if !errors.Is(err, ErrCommandFailed) {
		t.Fatalf("err %v does not wrap ErrCommandFailed", err)
	}
	var cerr *CommandError
	if !errors.As(err, &cerr) {
		t.Fatalf("err %v does not unwrap to *CommandError", err)
	}
	if cerr.Message != "boom" {
		t.Fatalf("CommandError.Message = %q, want %q", cerr.Message, "boom")
	}
	if got, want := fs.callCount(), 2; got != want {
		t.Fatalf("RunLine called %d times after failure, want %d", got, want)
	}
	if !strings.Contains(err.Error(), "t:2") {
		t.Fatalf("error %q does not mention source:line", err.Error())
	}
}

func TestRunReportsSourceAndLineOnParseError(t *testing.T) {
	fs := &fakeServer{}
	script := strings.NewReader("ok\nbad \"unterminated\nignored\n")
	err := Run(context.Background(), fs.dial, script, RunOptions{Source: "src"})
	if err == nil {
		t.Fatalf("Run: expected parse error")
	}
	if !errors.Is(err, ErrUnterminatedQuote) {
		t.Fatalf("err %v does not wrap ErrUnterminatedQuote", err)
	}
	if !strings.Contains(err.Error(), "src:2") {
		t.Fatalf("error %q does not mention src:2", err.Error())
	}
	if got, want := fs.callCount(), 1; got != want {
		t.Fatalf("RunLine called %d times, want %d", got, want)
	}
}

func TestRunLineRejectsEmptyArgv(t *testing.T) {
	fs := &fakeServer{}
	err := RunLine(context.Background(), fs.dial, nil)
	if err == nil {
		t.Fatalf("RunLine([]): expected error")
	}
	if fs.callCount() != 0 {
		t.Fatalf("dial called for empty argv")
	}
}
