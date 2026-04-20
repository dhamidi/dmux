// Package pane is a single PTY paired with a terminal emulator.
//
// This is the fundamental unit of terminal emulation in dmux. Every
// shell running under dmux is one Pane.
//
// # Boundary
//
// A Pane owns:
//
//   - one [pty.PTY] (child process + byte pipe) — held via the [pty.PTY]
//     interface, never a concrete type
//   - one [Terminal] (escape-sequence parser, cell grid, scrollback,
//     modes) — held via the local [Terminal] interface; the concrete
//     implementation is a go-libghostty adapter constructed outside
//     this package
//   - one [KeyEncoder] (encode [keys.Key] → escape bytes for the PTY)
//   - one [MouseEncoder] (encode [keys.MouseEvent] → escape bytes for the PTY)
//   - a goroutine that copies PTY output into the Terminal
//
// All four dependencies are accepted via [Config] and accessed only
// through their interfaces, so tests can pass fakes ([FakeTerminal],
// [FakeKeyEncoder], [FakeMouseEncoder], [pty.FakePTY]) with no OS
// resources required.
//
// # Pane interface
//
// The public surface of a running pane is the [Pane] interface:
//
//	type Pane interface {
//	    ID() PaneID
//	    Title() string
//	    Write(data []byte) error
//	    SendKey(key keys.Key) error
//	    SendMouse(ev keys.MouseEvent) error
//	    Resize(cols, rows int) error
//	    Snapshot() CellGrid
//	    Close() error
//	}
//
// [New] returns a [Pane]; the concrete struct is unexported.
//
// # Terminal interface
//
// [Terminal] exposes only what this package needs from a VT terminal
// emulator:
//
//	type Terminal interface {
//	    Write(p []byte) (int, error)     // feed raw PTY output
//	    Resize(cols, rows int) error     // update grid dimensions
//	    Title() (string, error)          // current OSC-2 title
//	    Snapshot() CellGrid             // immutable viewport snapshot
//	    Close()
//	}
//
// The concrete go-libghostty adapter is constructed by the caller and
// passed in via [Config.Term].
//
// # KeyEncoder and MouseEncoder interfaces
//
// [KeyEncoder] and [MouseEncoder] encode user-visible event types from
// [internal/keys] into the byte sequences expected by the child process:
//
//	type KeyEncoder interface {
//	    Encode(key keys.Key) ([]byte, error)
//	    Close()
//	}
//
//	type MouseEncoder interface {
//	    Encode(ev keys.MouseEvent) ([]byte, error)
//	    Close()
//	}
//
// Concrete adapters over go-libghostty are constructed outside this
// package and injected via [Config].
//
// # Key routing
//
// [Pane.SendKey] encodes the key via [KeyEncoder] and writes the result
// to the PTY. [Pane.SendMouse] does the same for mouse events. Pane
// modes (copy-mode, tree-mode, etc.) are owned by their callers, not by
// this package — callers decide whether to deliver events to the pane or
// to a mode overlay.
//
// # Output copying
//
// A background goroutine reads bytes from the PTY and feeds them to
// [Terminal.Write]. The goroutine exits when [pty.PTY.Read] returns
// a non-nil error (typically [io.EOF] after [Pane.Close]).
//
// # Cell grid
//
// [CellGrid] is a row-major snapshot of the visible viewport:
//
//	type CellGrid struct {
//	    Rows  int
//	    Cols  int
//	    Cells []Cell   // Cells[row*Cols+col]
//	}
//
// [Pane.Snapshot] delegates to [Terminal.Snapshot] and returns an
// immutable grid for compositing by the render layer.
//
// # Non-goals
//
// Knows nothing about other panes, windows, layouts, sessions, or
// clients. Does not render itself to the real terminal — it produces
// a [CellGrid]; package render composes it with other panes.
package pane
