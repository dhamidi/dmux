// Package pane is one VT + PTY pair: a single terminal emulator
// instance attached to a single child process, owned by its own
// goroutine.
//
// # The pane goroutine
//
// Each pane runs in its own goroutine. That goroutine exclusively
// owns the pane's vt.Terminal, vt.KeyEncoder, and current viewport.
// Heavy pty output in one pane runs vt.Feed on that pane's
// goroutine without blocking any other pane or the server's main
// loop. This is the central reason the design uses goroutines per
// pane rather than a single server-loop owning every vt.
//
// Topology per pane:
//
//	pty.Read goroutine        ----[bytes ch]--->
//	                                              \
//	                                               pane goroutine (owns vt)
//	                                              /        |
//	server main goroutine     ----[control ch]---/         |
//	                                                       v
//	                                              [snapshots ch]----> server main goroutine
//
// The pty.Read goroutine is a thin wrapper that does
//
//	for { n, err := pty.Read(buf); if err != nil { return }; bytesCh <- buf[:n] }
//
// It exists separately because Read blocks; it can't be part of the
// pane goroutine's select loop directly.
//
// # Pane goroutine loop
//
// The pane goroutine's loop:
//
//	for {
//	    select {
//	    case b := <-bytesCh:
//	        term.Feed(b)
//	        markDirty()
//	    case cmd := <-controlCh:
//	        switch c := cmd.(type) {
//	        case sendKey:    pty.Write(term.KeyEncoder().Encode(c.Event))
//	        case sendMouse:  pty.Write(encodeMouse(c.Event, term.Modes()))
//	        case resize:     term.Resize(c.Cols, c.Rows); pty.Resize(c.Cols, c.Rows); resetViewportToLive()
//	        case snapshot:   c.Reply <- term.Snapshot(viewport)
//	        case capsProbe:  reply := buildProbeReply(c.Probe, attachedProfiles)
//	                         pty.Write(reply)
//	        case attachedClientsChanged:
//	                         attachedProfiles = c.Profiles
//	        }
//	    case <-renderTick.C:
//	        if dirty { pushSnapshotToServer(); dirty = false }
//	    case <-ctx.Done():
//	        return
//	    }
//	}
//
// `attachedProfiles` is a local snapshot of which client profiles
// are currently attached, pushed by the server when clients
// attach/detach. The pane goroutine reads it without locking because
// it owns the field.
//
// # Render rate limiting
//
// Snapshot work is bounded per pane by `renderTick`. M1 default is
// ~16ms (60fps); the rate limit is configurable per pane in M5.
// Without this, a pane producing 100MB/s of output would generate
// thousands of snapshots per second and overwhelm the server's main
// loop with redundant render work.
//
// The dirty flag means snapshots are only produced when there is
// new content to render. A quiescent pane produces zero snapshots.
//
// # Communication with the server
//
// All cross-goroutine communication is through channels owned by
// the Pane:
//
//	type Pane struct {
//	    ID         PaneID
//	    bytesCh    chan []byte         // pty.Read -> pane goroutine
//	    controlCh  chan paneControl    // server -> pane goroutine
//	    snapshotCh chan vt.Grid        // pane goroutine -> server (push-on-dirty)
//	    exitedCh   chan pty.ExitStatus // pane goroutine -> server (one-shot)
//	}
//
// The server selects on `snapshotCh` and `exitedCh` from every pane
// (multiplexed via a fan-in goroutine into the main events channel).
//
// # Viewport ownership
//
// The pane owns the viewport. When two clients are attached and a
// user enters copy mode to scroll back, both clients see the
// scrollback position change. Per-client state (overlay positions,
// menu state) lives on the server's Client struct, not here.
//
// In M1 the viewport is permanently term.LiveViewport(). M4 adds
// methods to change it for copy mode (SetViewport, EnterCopyMode,
// ExitCopyMode); these are queued on the same controlCh.
//
// Resize invalidates vt.LineRef values; the pane resets the viewport
// to LiveViewport on every Resize to avoid pointing at dead history.
//
// # Resize ordering: vt first, then pty
//
// The control-message handler for `resize` calls `term.Resize`
// before `pty.Resize`. This matches tmux's screen_resize-then-ioctl
// order and is the only safe ordering:
//
//	vt first, pty second  (correct)
//	  vt has new dimensions; ready for new-sized output.
//	  ioctl(TIOCSWINSZ) fires; shell gets SIGWINCH; emits new
//	  layout; new-sized output arrives at new-sized vt. No corruption.
//
//	pty first, vt second  (wrong)
//	  ioctl fires; shell immediately starts emitting new-sized
//	  output; output hits still-old vt; briefly corrupts the grid
//	  until the vt.Resize call catches up.
//
// The window between pty.Resize completing and vt.Resize completing
// is usually microseconds, but there is no guarantee the shell is
// slow about responding to SIGWINCH, and the pane goroutine may be
// preempted between the two calls.
//
// After both resizes, any bytes already buffered in the bytesCh
// from the pty.Read goroutine were written by the shell at the OLD
// size. Feeding them into the new-sized vt is fine (the bytes
// aren't size-aware) but the vt treats them as new-sized output. In
// practice this is a non-issue: TUI apps respond to SIGWINCH by
// emitting a full redraw, which overwrites stale pre-resize
// content.
//
// # Capability mediation
//
// The pane's app may probe the terminal: "do you support kitty
// graphics, sixel, OSC 8, KKP level 3?" The pane goroutine answers
// using its `attachedProfiles` snapshot — the most-restrictive
// profile across attached clients. The probe-recognition happens
// inside vt.Terminal (via libghostty-vt's OSC parser); the reply is
// composed by the pane goroutine and written to the pty input.
//
// Capabilities are sticky for the session's lifetime in M1. If a
// Ghostty client and a Windows Terminal client attach simultaneously,
// answers are conservative for both. When clients leave, the answers
// do not retroactively widen — the pane's app cannot be told "your
// previous capability answers were too pessimistic." This matches
// what every terminal protocol allows.
//
// # External interface (server-facing)
//
//	Open(ctx context.Context, cfg Config) (*Pane, error)
//	(*Pane) SendKey(keys.Event)             // sends on controlCh
//	(*Pane) SendMouse(termin.MouseEvent)    // sends on controlCh
//	(*Pane) Resize(cols, rows int)          // sends on controlCh
//	(*Pane) Snapshot(reply chan<- vt.Grid)  // sends on controlCh; non-blocking
//	(*Pane) AttachedClients(profiles []termcaps.Profile)  // sends on controlCh
//	(*Pane) Snapshots() <-chan vt.Grid      // server consumes pushed snapshots
//	(*Pane) Exited() <-chan pty.ExitStatus  // one-shot signal
//	(*Pane) Close() error                   // cancels ctx, waits for goroutines
//
// External callers never touch vt or pty directly. Every operation
// goes through the pane.
//
// # Scope boundary
//
// No layout (internal/session). No key-binding dispatch
// (internal/server). No rendering (internal/termout). No copy-mode
// key dispatch (cmd/copymode in M4 sends actions through controlCh).
//
// # Corresponding tmux code
//
// tmux's struct window_pane plus the pty / input glue from window.c,
// spawn.c, and input.c. tmux serializes everything through libevent
// callbacks on a single thread; we get the same single-writer
// guarantee per terminal via the per-pane goroutine, but heavy panes
// no longer block other panes.
package pane
