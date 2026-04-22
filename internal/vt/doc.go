// Package vt embeds libghostty-vt via wazero and exposes a Go-friendly
// interface for dmux.
//
// # Wazero hosting
//
// libghostty-vt has an official, supported wasm32-freestanding build
// target. The Ghostty repository builds it with
//
//	zig build -Demit-lib-vt -Dtarget=wasm32-freestanding -Doptimize=ReleaseSmall
//
// per AGENTS.md in the upstream repo. No emscripten is required.
// Production consumers (coder/ghostty-web for browser-based xterm.js
// replacement, the Dart bindings package) already ship wasm builds.
//
// A single wazero.Runtime is created at server startup and shared by
// all panes. Each pane holds one GhosttyTerminal handle plus one
// GhosttyKeyEncoder handle inside that runtime. The wasm plumbing is
// not visible outside this package.
//
// # Concurrency contract
//
// `Terminal` is not safe for concurrent use. It is owned by exactly
// one goroutine — the pane goroutine that holds it (see
// internal/pane). All Feed, Resize, Snapshot, KeyEncoder.Encode, and
// other state-touching calls happen on that goroutine. Consumers
// outside the pane (the server's main loop, termout) never call
// Terminal methods directly; they go through the pane's RPC
// channels.
//
// This is enforced by convention, not by locks. Locking inside the
// wasm boundary would serialize the wasm instance unnecessarily —
// the per-pane goroutine model already gives us "one writer per
// terminal" without contention.
//
// # Terminal interface
//
//	Feed(b []byte)                  // pane-app output -> terminal state
//	Resize(cols, rows int)
//
//	// Viewport and snapshot
//	LiveViewport() Viewport          // viewport over the current live screen
//	Snapshot(v Viewport) Grid        // reify a viewport into a Go Grid
//
//	// Monotonic line addressing
//	OldestLine() LineRef
//	NewestLine() LineRef
//	HistoryLen() int
//
//	// Non-grid terminal state
//	Cursor() Cursor                  // live-screen cursor; not the copy-mode cursor
//	Modes() Modes                    // KKP flags, alt-screen, app cursor keys, etc.
//	Hyperlinks() map[ID]URL
//	Graphics() []Placement
//	Dirty() DirtyRegion              // scoped to the live viewport
//
//	// Per-tick side channels
//	TakeOSC52() []byte
//	TakeBell() bool
//
//	// Input encoding
//	KeyEncoder() KeyEncoder          // encoder synced to this terminal's modes
//
//	Close()
//
//	type KeyEncoder interface {
//	    Encode(keys.Event) []byte
//	    Close()
//	}
//
// # Line addressing
//
// A terminal's lines form a monotonic sequence. Each line
// libghostty-vt writes (either into the live screen or by scrolling
// one off the top) has a stable identifier, LineRef, implemented as
// an opaque uint64. New lines increment the counter. Lines dropped
// from the bottom of scrollback leave their LineRefs invalid — but
// refs are never reused, so a stored ref either resolves to its
// original content or is permanently gone.
//
// A Viewport is (TopLine, Rows, Cols):
//
//	type Viewport struct {
//	    TopLine LineRef
//	    Rows    int
//	    Cols    int
//	}
//
// Snapshot(v) reads the Rows lines starting at v.TopLine and returns
// a Grid:
//
//	type Grid struct {
//	    TopLine LineRef
//	    Rows    int
//	    Cols    int
//	    Lines   []Line
//	}
//
// LiveViewport() returns the viewport over the current live screen.
//
// # What M1 uses
//
// M1 only ever calls LiveViewport() + Snapshot(). The history
// methods and arbitrary-viewport snapshots exist in the interface
// but go unused until copy mode lands in M4. Adding copy mode is a
// matter of calling methods that already exist, not adding new ones.
//
// # Bounded reads across the wasm boundary
//
// Grid content lives in the libghostty-vt wasm instance's linear
// memory. Reads across the wasm<->Go boundary are always
// viewport-scoped — one screen's worth of cells, tens of KB per
// Snapshot. There is deliberately no "give me everything" method.
//
// Search over scrollback (a concern for M4) is exposed as a bounded
// incremental iterator:
//
//	type SearchState struct { ... }
//	Search(query string, from SearchState, maxLines int) (hits []Hit, next SearchState, done bool)
//
// # Alt screen and viewport
//
// When the pane's app enables the alternate screen (vim, less, man),
// the live screen is replaced with the alt-screen grid which has no
// scrollback of its own. LiveViewport() returns whatever grid is
// currently showing.
//
// # Resize invalidates LineRefs
//
// After Resize, libghostty-vt may reflow scrollback. LineRef values
// held across a Resize may become invalid or point to different
// content. Callers that persist a LineRef must resnapshot or drop
// their reference after Resize.
//
// # Why the encoder lives on Terminal
//
// The pane-side encoder must track the pane app's current keyboard
// mode (legacy / modifyOtherKeys / KKP with specific flags). That
// state lives inside the ghostty terminal instance and is updated by
// application output (Feed). Exposing the encoder via Terminal means
// callers cannot desynchronize the two.
//
// # Scope boundary
//
//   - No input parsing (see internal/termin).
//   - No rendering to a real terminal (see internal/termout).
//   - No pty I/O (see internal/pty). Package pane wires them together.
//   - No copy-mode UI. The scrollback-reading primitives live here;
//     copy-mode state and key handling live in internal/pane and
//     internal/cmd/copymode (M4).
//
// # Corresponding tmux code
//
// tmux's input.c, screen.c, grid.c, and utf8.c (the state-machine
// half). Their combined responsibility is what libghostty-vt gives us.
package vt
