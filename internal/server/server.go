package server

import (
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/socket"
	"github.com/dhamidi/dmux/internal/xio"
)

// Run is the M1 walking-skeleton server entry point. It binds the
// AF_UNIX socket at path, accepts exactly one client, performs the
// Identify / CommandList / Bye exchange, and returns.
//
// This is a stub. The real server (see doc.go) runs a
// select-over-events main loop with per-client and per-pane
// goroutines. Swapping this out will not change the frame-level
// protocol, which is what the walking skeleton exercises.
func Run(path string) error {
	l, err := socket.Listen(path)
	if err != nil {
		return fmt.Errorf("server: listen: %w", err)
	}
	defer l.Close()

	conn, err := l.Accept()
	if err != nil {
		return fmt.Errorf("server: accept: %w", err)
	}
	defer conn.Close()

	return handle(conn)
}

// handle runs one client's frame exchange. The skeleton accepts
// Identify first (per the proto contract), then loops over
// CommandList and Bye. CommandList returns StatusOk for every
// command; Bye triggers a clean Exit and returns nil.
func handle(conn net.Conn) error {
	r := xio.NewReader(conn)
	w := xio.NewWriter(conn)

	first, err := r.ReadFrame()
	if err != nil {
		return fmt.Errorf("server: read Identify: %w", err)
	}
	if first.Type() != proto.MsgIdentify {
		// Protocol violation. Best-effort Exit, then close.
		_ = w.WriteFrame(&proto.Exit{
			Reason:  proto.ExitProtocolError,
			Message: "expected Identify as first frame, got " + first.Type().String(),
		})
		return fmt.Errorf("server: protocol error: first frame was %s", first.Type())
	}

	for {
		f, err := r.ReadFrame()
		if err != nil {
			if errors.Is(err, io.EOF) {
				// Client went away without Bye. Not an error
				// for the skeleton; a real server would log as
				// a lost-connection event.
				return nil
			}
			return fmt.Errorf("server: read frame: %w", err)
		}

		switch m := f.(type) {
		case *proto.CommandList:
			for _, cmd := range m.Commands {
				res := &proto.CommandResult{
					ID:     cmd.ID,
					Status: proto.StatusOk,
				}
				if err := w.WriteFrame(res); err != nil {
					return fmt.Errorf("server: write CommandResult: %w", err)
				}
			}
		case *proto.Bye:
			if err := w.WriteFrame(&proto.Exit{Reason: proto.ExitDetached}); err != nil {
				return fmt.Errorf("server: write Exit: %w", err)
			}
			return nil
		default:
			// Walking skeleton: ignore Input/Resize/CapsUpdate
			// silently. Real server dispatches to panes.
		}
	}
}
