// Package session is the object model: the Server aggregate and all
// the nouns inside it — Session, Window, Winlink, Client, PasteBuffer.
//
// # Boundary
//
// Pure state with mutating methods. Owns no goroutines, does no I/O,
// imports nothing from Tier 3+.
//
//	type Server struct {
//	    Sessions   map[SessionID]*Session
//	    Clients    map[ClientID]*Client
//	    Buffers    *BufferStack
//	    Options    *options.Store
//	    Env        *Environ
//	    Hooks      *HookTable
//	}
//
//	type Session struct {
//	    ID        SessionID
//	    Name      string
//	    Windows   []*Winlink      // ordered, may contain same *Window
//	                              //   twice (session groups)
//	    Options   *options.Store  // parent = Server.Options
//	    Env       *Environ
//	    Current   *Winlink
//	}
//
//	type Window struct {
//	    ID        WindowID
//	    Name      string
//	    Layout    *layout.Tree
//	    Panes     map[layout.LeafID]Pane    // interface, not pane.Pane
//	    Active    layout.LeafID
//	    Options   *options.Store
//	}
//
//	type Pane interface {
//	    Title() string
//	    Snapshot(rs *render.RenderState)
//	    Resize(cols, rows int)
//	    Close() error
//	}
//
// Window holds Pane-the-interface so session does not import pane.
// Production wires in *pane.Pane (which satisfies the interface);
// tests wire in a struct literal.
//
//	type Client struct {
//	    ID        ClientID
//	    Session   *Session
//	    Size      render.Size
//	    TTY       string
//	    Term      string          // $TERM
//	    Features  FeatureSet
//	    KeyTable  string          // "root" unless inside a prefix / copy-mode
//	    Overlays  []modes.ClientOverlay
//	    Env       *Environ        // captured at attach
//	    Cwd       string
//	}
//
// # Context implementation
//
// Server, Session, Window, and Pane each satisfy format.Context by
// exposing Lookup and Children. `#{session_name}` works because Session
// returns its Name for "session_name"; `#{W:#{window_name}}` works
// because Session returns its Winlinks for Children("W").
//
// # I/O surfaces
//
// None. Pure state, no goroutines, no clocks. Mutations are synchronous
// method calls.
//
// # In isolation
//
// Buildable in a test without any I/O: construct a Server, add
// Sessions, Windows, and stub Panes, assert on the model. Serialization
// helpers let tests round-trip state for layout and reattach scenarios.
//
// # Non-goals
//
// Not an event loop. Mutations happen via commands (Tier 3) driven by
// the server loop (Tier 4). Not a persistence layer — state is in memory
// only unless a future `session-persistence` feature is added elsewhere.
package session
