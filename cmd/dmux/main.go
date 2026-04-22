package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dhamidi/dmux/internal/platform"
	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/server"
	"github.com/dhamidi/dmux/internal/socket"
	"github.com/dhamidi/dmux/internal/sockpath"
	"github.com/dhamidi/dmux/internal/xio"
)

func main() {
	// Branch before flag parsing: the server child is re-exec'd
	// with the parent's argv, so interpreting flags on this side
	// would just mean flag.Parse() failures on values meant for
	// the client invocation.
	if path, ok := platform.IsServerChild(); ok {
		if err := server.Run(path); err != nil {
			// stderr is /dev/null in the detached child. Exit
			// with a non-zero code so at least the wait status
			// carries signal. A log file lands in a later pass.
			os.Exit(1)
		}
		return
	}

	if err := clientMain(); err != nil {
		fmt.Fprintf(os.Stderr, "dmux: %v\n", err)
		os.Exit(1)
	}
}

func clientMain() error {
	socketFlag := flag.String("S", "", "socket path (overrides -L and $DMUX)")
	labelFlag := flag.String("L", "", "socket label (default: \"default\")")
	flag.Parse()

	path, err := sockpath.Resolve(sockpath.Options{
		SocketPath: *socketFlag,
		Label:      *labelFlag,
	})
	if err != nil {
		return err
	}

	// sockpath does not create the parent dir (its doc.go calls
	// this out explicitly). The server needs it to exist before
	// bind; create it with 0700 to match sockpath's tmpdir check.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}

	conn, err := socket.DialOrStart(path, func() error {
		return platform.SpawnServer(path)
	})
	if err != nil {
		return err
	}
	defer conn.Close()

	return exchange(conn)
}

// exchange runs the M1 walking-skeleton frame sequence:
//
//	-> Identify
//	-> CommandList{attach-session}
//	<- CommandResult
//	-> Bye
//	<- Exit
//
// Real client goroutine structure (stdin/output/resize) arrives
// once internal/client is built out.
func exchange(conn interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
}) error {
	w := xio.NewWriter(conn)
	r := xio.NewReader(conn)

	if err := w.WriteFrame(&proto.Identify{
		ProtocolVersion: proto.ProtocolVersion,
	}); err != nil {
		return fmt.Errorf("send Identify: %w", err)
	}

	if err := w.WriteFrame(&proto.CommandList{
		Commands: []proto.Command{{ID: 1, Argv: []string{"attach-session"}}},
	}); err != nil {
		return fmt.Errorf("send CommandList: %w", err)
	}

	f, err := r.ReadFrame()
	if err != nil {
		return fmt.Errorf("read CommandResult: %w", err)
	}
	cr, ok := f.(*proto.CommandResult)
	if !ok {
		return fmt.Errorf("expected CommandResult, got %s", f.Type())
	}
	fmt.Printf("result: id=%d status=%s message=%q\n", cr.ID, cr.Status, cr.Message)

	if err := w.WriteFrame(&proto.Bye{}); err != nil {
		return fmt.Errorf("send Bye: %w", err)
	}

	f, err = r.ReadFrame()
	if err != nil {
		return fmt.Errorf("read Exit: %w", err)
	}
	ex, ok := f.(*proto.Exit)
	if !ok {
		return fmt.Errorf("expected Exit, got %s", f.Type())
	}
	fmt.Printf("exit: reason=%s message=%q\n", ex.Reason, ex.Message)
	return nil
}
