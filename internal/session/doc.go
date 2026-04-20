// Package session is the object model: the Server aggregate and all
// the nouns inside it — Session, Window, Winlink, Client, PasteBuffer.
//
// # Boundary
//
// Pure state with mutating methods. Owns no goroutines, does no I/O,
// and imports nothing from Tier 3+. The only internal packages imported
// are:
//
//   - [github.com/dhamidi/dmux/internal/layout] (Tier 1) — for
//     [layout.Tree] (window pane layout) and [layout.LeafID] (pane
//     identity, re-exported as [PaneID]).
//   - [github.com/dhamidi/dmux/internal/options] (Tier 0) — for
//     [options.Store] (hierarchical key-value options).
//   - [github.com/dhamidi/dmux/internal/format] (Tier 2, no reverse
//     dependency) — only to name the [format.Context] return type in
//     the Children methods.
//
// # Interfaces defined by this package
//
// To avoid coupling session to concrete Tier-1/3 implementations, two
// narrow interfaces are declared here:
//
//	// Pane is what session requires of a running pane.
//	// The concrete type lives in internal/pane.
//	type Pane interface {
//	    Title() string
//	    Resize(cols, rows int) error
//	    Close() error
//	}
//
//	// Overlay is what session requires of a client overlay / mode.
//	// Concrete types live in internal/modes.
//	type Overlay interface {
//	    OverlayName() string
//	}
//
// # Aggregate types
//
//	type Server struct {
//	    Sessions   map[SessionID]*Session
//	    Clients    map[ClientID]*Client
//	    Buffers    *BufferStack
//	    Options    *options.Store
//	    Env        Environ
//	    Hooks      *HookTable
//	}
//
//	type Session struct {
//	    ID        SessionID
//	    Name      string
//	    Windows   []*Winlink       // ordered; duplicate Window pointers allowed
//	    Options   *options.Store   // parent = Server.Options
//	    Env       Environ
//	    Current   *Winlink
//	}
//
//	type Window struct {
//	    ID        WindowID
//	    Name      string
//	    Layout    *layout.Tree
//	    Panes     map[PaneID]Pane  // PaneID = layout.LeafID
//	    Active    PaneID
//	    Options   *options.Store
//	}
//
//	type Client struct {
//	    ID        ClientID
//	    Session   *Session         // nil when detached
//	    Size      Size             // defined locally: {Cols, Rows int}
//	    TTY       string
//	    Term      string           // $TERM
//	    Features  FeatureSet
//	    KeyTable  string           // "root" unless inside a prefix / copy-mode
//	    Overlays  []Overlay        // interface; concrete types from internal/modes
//	    Env       Environ          // captured at attach
//	    Cwd       string
//	}
//
// # Context implementation
//
// Server, Session, Winlink, Window, and Client each satisfy [format.Context]
// by exposing Lookup and Children methods. Examples:
//
//   - Server.Lookup("session_count") → number of sessions
//   - Session.Lookup("session_name") → session name
//   - Winlink.Lookup("window_index") → index within session
//   - Window.Lookup("window_panes") → pane count
//   - Client.Lookup("client_width") → terminal width in columns
//
// # In isolation
//
// Buildable in a test without any I/O: construct a Server, add
// Sessions and Windows, attach mock Panes (implement the Pane interface),
// and assert on the model state. No real PTYs, networks, or terminals
// are required.
//
// # Non-goals
//
// Not an event loop. Mutations happen via commands (Tier 3) driven by
// the server loop (Tier 4). Not a persistence layer — state is in memory
// only unless a future session-persistence feature is added elsewhere.
package session
