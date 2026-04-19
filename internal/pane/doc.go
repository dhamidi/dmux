// Package pane is a single PTY paired with a libghostty-vt Terminal.
//
// This is the fundamental unit of terminal emulation in dmux. Every
// shell running under dmux is one Pane.
//
// # Boundary
//
// A Pane owns:
//
//   - one pty.PTY (the interface, not the concrete type — passed in by
//     the caller, who chose to start a real PTY or a fake one)
//   - one libghostty.Terminal (escape-sequence parser, grid, scrollback,
//     modes) — concrete because libghostty IS the terminal emulator
//     this package is built around
//   - one libghostty.KeyEncoder and libghostty.MouseEncoder
//   - a goroutine that copies PTY output into the Terminal
//   - the current PaneMode, if any (see package modes)
//
// Public surface:
//
//	New(cfg Config) (*Pane, error)
//
//	type Config struct {
//	    PTY  pty.PTY        // interface; caller decides how to spawn
//	    Cols, Rows int
//	    Title      string
//	}
//
//	(*Pane).SendKey(key Key)               encode + write to PTY
//	(*Pane).SendMouse(ev MouseEvent)       encode + write to PTY
//	(*Pane).Resize(cols, rows)             update both PTY and Terminal
//	(*Pane).Snapshot(rs *RenderState)      populate a render state
//	(*Pane).SetMode(m PaneMode)            enter copy-mode, tree, etc.
//	(*Pane).Title() string
//	(*Pane).Close() error
//
// # I/O surfaces
//
//   - Spawns one goroutine that reads from the injected PTY and feeds
//     bytes into libghostty.
//   - Writes to the same PTY in response to SendKey, SendMouse, and
//     libghostty's write_pty effect.
//
// No environment reads, no filesystem reads, no network. PTY ownership
// (process spawn, fd lifecycle) lives in package pty.
//
// The libghostty Terminal's write_pty effect is wired to a callback
// inside this package that writes the bytes back to the PTY. That's
// how device-attribute queries, DSR, XTWINOPS, etc. get answered.
//
// # Mode dispatch
//
// If CurrentMode is nil, SendKey encodes the key and writes it to the
// PTY. If CurrentMode is non-nil, SendKey calls the mode's Key handler
// instead and the mode decides what happens — nothing is sent to the
// shell. Either way, the shell continues running and output continues
// being parsed into the Terminal in the background.
//
// # In isolation
//
// A small example opens one pane running $SHELL, feeds keystrokes
// from os.Stdin, and prints rendered cells to os.Stdout every 100ms.
// A single-pane toy terminal in ~100 lines.
//
// # Non-goals
//
// Knows nothing about other panes, windows, layouts, sessions, or
// clients. Does not render itself to the real terminal — it produces
// render state; package render composes it with others and draws.
package pane
