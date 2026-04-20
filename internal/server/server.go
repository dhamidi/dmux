package server

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dhamidi/dmux/internal/command"
	_ "github.com/dhamidi/dmux/internal/command/builtin"
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/pty"
	"github.com/dhamidi/dmux/internal/render"
	"github.com/dhamidi/dmux/internal/session"
	"github.com/dhamidi/dmux/internal/shell"
)

// protoVersion is the wire protocol version this server implements.
const protoVersion uint16 = 1

// Clock is a function that returns the current time.
// Inject a deterministic clock in tests; use time.Now in production.
type Clock func() time.Time

// Config holds all I/O dependencies and settings for the dmux server.
// Every field is explicit: Run never calls os.Stderr, os.Getenv,
// time.Now, or signal.Notify directly.
type Config struct {
	// Listener accepts incoming client connections. The caller opens the
	// socket (Unix-domain or otherwise) before constructing Config.
	// Tests may use a net.Pipe-backed listener.
	Listener net.Listener

	// Log is the destination for server diagnostic output.
	// Defaults to io.Discard if nil.
	Log io.Writer

	// Signals receives OS signals. Any received value triggers a graceful
	// shutdown. Wire to os/signal.Notify in cmd/; send synthetic values
	// directly in tests to simulate SIGTERM or SIGHUP.
	Signals <-chan os.Signal

	// Now returns the current time for timer logic (debounce intervals,
	// status ticks, etc.). Defaults to time.Now if nil.
	Now Clock

	// ConfigFile, if non-empty, is sourced via the source-file command
	// after the server initialises its default bindings and options.
	// A missing or unreadable file is silently ignored.
	ConfigFile string

	// OnDirty, when non-nil, is called after a client is marked dirty
	// for redraw. Tests use this hook to observe redraw scheduling
	// without a full rendering layer.
	OnDirty func(id session.ClientID)

	// State, if non-nil, is used as the initial server state instead of
	// creating a new empty Server. Primarily useful in tests that need
	// to pre-populate sessions, windows, or panes.
	State *session.Server
}

// clientDecoder bundles a write-side buffer with the keys.Decoder that reads
// from it. Writing raw bytes to buf makes them available to dec.Next().
type clientDecoder struct {
	buf *bytes.Buffer
	dec *keys.Decoder
}

func newClientDecoder() *clientDecoder {
	buf := &bytes.Buffer{}
	return &clientDecoder{buf: buf, dec: keys.NewDecoder(buf)}
}

// srv is the running server state.
type srv struct {
	cfg        Config
	state      *session.Server
	store      command.Server
	mutator    command.Mutator
	queue      *command.Queue
	log        *log.Logger
	mu         sync.Mutex
	conns      map[session.ClientID]*clientConn
	decoders   map[session.ClientID]*clientDecoder
	prevFrames map[session.ClientID]render.CellGrid
	nextID     uint64
	done       chan struct{}
	once       sync.Once // ensures done is closed at most once
	wg         sync.WaitGroup
}

// clientConn is the server-side view of one connected client.
type clientConn struct {
	id      session.ClientID
	netConn net.Conn
	client  *session.Client
	dirty   chan struct{} // buffered(1); written to when a redraw is needed
}

// Run starts the dmux server and blocks until shutdown.
//
// Run never calls os.Stderr, os.Getenv, time.Now, or signal.Notify
// directly. All I/O flows through the Config fields.
func Run(cfg Config) error {
	if cfg.Log == nil {
		cfg.Log = io.Discard
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	s := &srv{
		cfg:        cfg,
		log:        log.New(cfg.Log, "server: ", 0),
		conns:      make(map[session.ClientID]*clientConn),
		decoders:   make(map[session.ClientID]*clientDecoder),
		prevFrames: make(map[session.ClientID]render.CellGrid),
		done:       make(chan struct{}),
	}
	if cfg.State != nil {
		s.state = cfg.State
	} else {
		s.state = session.NewServer()
	}
	s.store = newServerStore(s.state)
	s.mutator = newServerMutator(s.state, s.triggerDone,
		func(id session.ClientID) (*clientConn, bool) {
			s.mu.Lock()
			defer s.mu.Unlock()
			cc, ok := s.conns[id]
			return cc, ok
		},
		func(cc *clientConn) { s.markDirty(cc) },
		func(cfg pane.Config) (session.Pane, error) {
			shellPath := shell.Default(os.LookupEnv, func(p string) bool {
				_, err := os.Stat(p)
				return err == nil
			})
			ptyDev, err := pty.Open(shellPath, nil, pty.Size{Rows: 24, Cols: 80})
			if err != nil {
				return nil, fmt.Errorf("new-pane: open pty: %w", err)
			}
			cfg.PTY = ptyDev
			cfg.Term = &pane.FakeTerminal{}
			cfg.Keys = &pane.FakeKeyEncoder{}
			cfg.Mouse = &pane.FakeMouseEncoder{}
			return pane.New(cfg)
		},
	)
	s.queue = command.NewQueue()

	loadDefaultOptions(s)

	if err := loadDefaultBindings(s.mutator); err != nil {
		return fmt.Errorf("load default bindings: %w", err)
	}

	if cfg.ConfigFile != "" {
		var nilClientView command.ClientView
		command.Dispatch("source-file", []string{cfg.ConfigFile},
			s.store, nilClientView, s.queue, s.mutator)
		s.queue.Drain()
	}

	s.wg.Add(1)
	go s.acceptLoop()

	// Wait for a shutdown trigger: an OS signal from the caller, or an
	// internal request from a client sending MsgShutdown.
	if cfg.Signals != nil {
		select {
		case sig := <-cfg.Signals:
			s.log.Printf("received signal %v, shutting down", sig)
			s.triggerDone()
		case <-s.done:
			s.log.Printf("internal shutdown requested")
		}
	} else {
		<-s.done
	}

	return s.shutdown()
}

// triggerDone closes s.done exactly once (idempotent).
func (s *srv) triggerDone() {
	s.once.Do(func() { close(s.done) })
}

// shutdown closes the listener and all client connections, then waits
// for all goroutines to exit.
func (s *srv) shutdown() error {
	if err := s.cfg.Listener.Close(); err != nil {
		s.log.Printf("closing listener: %v", err)
	}
	s.mu.Lock()
	for _, c := range s.conns {
		c.netConn.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	return nil
}

// acceptLoop accepts connections from the listener until it is closed.
// When it exits it triggers the done channel so Run() unblocks even if
// the listener was closed externally.
func (s *srv) acceptLoop() {
	defer s.wg.Done()
	defer s.triggerDone()

	for {
		nc, err := s.cfg.Listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				// Normal shutdown: listener was closed intentionally.
			default:
				s.log.Printf("accept: %v", err)
			}
			return
		}
		s.wg.Add(1)
		go s.serveConn(nc)
	}
}

// serveConn handles one client connection from VERSION through disconnect.
func (s *srv) serveConn(nc net.Conn) {
	defer s.wg.Done()
	defer nc.Close()

	// Step 1: VERSION
	msgType, payload, err := proto.ReadMsg(nc)
	if err != nil {
		s.log.Printf("read version: %v", err)
		return
	}
	if msgType != proto.MsgVersion {
		s.log.Printf("expected VERSION, got %s", msgType)
		return
	}
	var vm proto.VersionMsg
	if err := vm.Decode(payload); err != nil {
		s.log.Printf("decode VERSION: %v", err)
		return
	}
	if vm.Version != protoVersion {
		em := proto.ExitMsg{Code: 1}
		_ = proto.WriteMsg(nc, proto.MsgExit, em.Encode())
		s.log.Printf("version mismatch: client=%d server=%d", vm.Version, protoVersion)
		return
	}

	// Step 2: IDENTIFY sequence
	client, err := s.readIdentify(nc)
	if err != nil {
		s.log.Printf("identify: %v", err)
		return
	}

	// Step 3: register client
	cc := &clientConn{
		id:      client.ID,
		netConn: nc,
		client:  client,
		dirty:   make(chan struct{}, 1),
	}
	s.mu.Lock()
	s.state.Clients[client.ID] = client
	s.conns[client.ID] = cc
	s.decoders[client.ID] = newClientDecoder()
	s.mu.Unlock()

	s.log.Printf("client %s attached (tty=%s term=%s)", client.ID, client.TTY, client.Term)

	// S6: auto-create an initial session when the first client attaches to a
	// server that has no sessions (mirrors tmux's startup behaviour).
	s.mu.Lock()
	noSessions := len(s.state.Sessions) == 0
	s.mu.Unlock()

	if noSessions {
		view, err := s.mutator.NewSession("") // name defaults to "0"
		if err == nil {
			_ = s.mutator.AttachClient(string(cc.id), view.ID)
		}
	}

	defer func() {
		s.mu.Lock()
		delete(s.conns, client.ID)
		delete(s.decoders, client.ID)
		s.state.DetachClient(client.ID)
		close(cc.dirty)
		s.mu.Unlock()
		s.log.Printf("client %s detached", client.ID)
	}()

	// Step 4: start per-client render goroutine, then enter message loop.
	s.wg.Add(1)
	go s.renderLoop(cc)

	s.clientLoop(cc)
}

// readIdentify reads IDENTIFY_* messages until IDENTIFY_DONE and returns
// a populated *session.Client. An optional RESIZE message after IDENTIFY_DONE
// sets the initial terminal size.
func (s *srv) readIdentify(nc net.Conn) (*session.Client, error) {
	s.mu.Lock()
	s.nextID++
	id := session.ClientID(fmt.Sprintf("c%d", s.nextID))
	s.mu.Unlock()

	client := session.NewClient(id)

	for {
		msgType, payload, err := proto.ReadMsg(nc)
		if err != nil {
			return nil, fmt.Errorf("reading identify: %w", err)
		}
		switch msgType {
		case proto.MsgIdentifyFlags:
			// Bitmask reserved for future flag handling.
		case proto.MsgIdentifyTerm:
			var m proto.IdentifyTermMsg
			if err := m.Decode(payload); err == nil {
				client.Term = m.Term
			}
		case proto.MsgIdentifyTerminfo:
			// Raw terminfo bytes; retained for future rendering use.
		case proto.MsgIdentifyTTYName:
			var m proto.IdentifyTTYNameMsg
			if err := m.Decode(payload); err == nil {
				client.TTY = m.TTYName
			}
		case proto.MsgIdentifyCWD:
			var m proto.IdentifyCWDMsg
			if err := m.Decode(payload); err == nil {
				client.Cwd = m.CWD
			}
		case proto.MsgIdentifyEnviron:
			var m proto.IdentifyEnvironMsg
			if err := m.Decode(payload); err == nil {
				client.Env = parseEnviron(m.Pairs)
			}
		case proto.MsgIdentifyClientPID:
			// PID available for future process management; not yet used.
		case proto.MsgIdentifyFeatures:
			var m proto.IdentifyFeaturesMsg
			if err := m.Decode(payload); err == nil {
				client.Features = session.FeatureSet(m.Features)
			}
		case proto.MsgIdentifyDone:
			return client, nil
		case proto.MsgResize:
			var m proto.ResizeMsg
			if err := m.Decode(payload); err == nil {
				client.Size = session.Size{Cols: int(m.Width), Rows: int(m.Height)}
			}
		default:
			// Unexpected message during identify; skip.
		}
	}
}

// parseEnviron converts "KEY=VALUE" strings into a session.Environ map.
func parseEnviron(pairs []string) session.Environ {
	env := make(session.Environ, len(pairs))
	for _, kv := range pairs {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				env[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	return env
}

// clientLoop reads and handles messages from a connected client until
// disconnect, DETACH, or server shutdown.
func (s *srv) clientLoop(cc *clientConn) {
	for {
		msgType, payload, err := proto.ReadMsg(cc.netConn)
		if err != nil {
			return
		}
		switch msgType {
		case proto.MsgDetach:
			return

		case proto.MsgResize:
			var m proto.ResizeMsg
			if err := m.Decode(payload); err == nil {
				s.mu.Lock()
				cc.client.Size = session.Size{Cols: int(m.Width), Rows: int(m.Height)}
				s.mu.Unlock()
				s.markDirty(cc)
			}

		case proto.MsgCommand:
			var cm proto.CommandMsg
			if err := cm.Decode(payload); err != nil {
				continue
			}
			if len(cm.Argv) == 0 {
				continue
			}
			s.mu.Lock()
			clientView, _ := s.store.GetClient(string(cc.client.ID))
			s.mu.Unlock()
			result := command.Dispatch(cm.Argv[0], cm.Argv[1:], s.store, clientView, s.queue, s.mutator)
			s.queue.Drain()
			if result.Err != nil {
				msg := proto.StdoutMsg{Data: []byte(result.Err.Error() + "\r\n")}
				_ = proto.WriteMsg(cc.netConn, proto.MsgStdout, msg.Encode())
			}
			if result.Output != "" {
				msg := proto.StdoutMsg{Data: []byte(result.Output)}
				_ = proto.WriteMsg(cc.netConn, proto.MsgStdout, msg.Encode())
			}
			s.markDirty(cc)

		case proto.MsgStdin:
			var sm proto.StdinMsg
			if err := sm.Decode(payload); err != nil {
				continue
			}
			rawBytes := sm.Data
			if len(rawBytes) == 0 {
				continue
			}

			s.mu.Lock()
			cd := s.decoders[cc.client.ID]
			if cd != nil {
				cd.buf.Write(rawBytes) //nolint:errcheck // bytes.Buffer.Write never fails
			}
			s.mu.Unlock()

			if cd == nil {
				continue
			}

			for {
				k, err := cd.dec.Next()
				if err != nil {
					break // io.EOF or decode error: no more keys in this payload
				}

				s.mu.Lock()
				tableName := cc.client.KeyTable
				if tableName == "" {
					tableName = "root"
				}
				table, hasTable := s.state.KeyTables.Get(tableName)
				var (
					boundCmd string
					isBound  bool
				)
				if hasTable {
					if rawCmd, found := table.Lookup(k); found {
						if cmdStr, isStr := rawCmd.(string); isStr && cmdStr != "" {
							boundCmd = cmdStr
							isBound = true
						}
					}
				}
				var activePane session.Pane
				if !isBound && cc.client.Session != nil && cc.client.Session.Current != nil {
					win := cc.client.Session.Current.Window
					if pane, ok := win.Panes[win.Active]; ok {
						activePane = pane
					}
				}
				clientView, _ := s.store.GetClient(string(cc.client.ID))
				s.mu.Unlock()

				if isBound {
					argv := strings.Fields(boundCmd)
					if len(argv) > 0 {
						command.Dispatch(argv[0], argv[1:], s.store, clientView, s.queue, s.mutator)
					}
				} else if activePane != nil {
					_ = activePane.Write(rawBytes)
				}
			}

			s.queue.Drain()
			s.markDirty(cc)

		case proto.MsgShutdown:
			s.log.Printf("client %s requested shutdown", cc.id)
			s.triggerDone()
			return
		}
	}
}

// markDirty schedules a redraw for cc (non-blocking, coalesced).
// It also calls cfg.OnDirty if set, for test observability.
func (s *srv) markDirty(cc *clientConn) {
	select {
	case cc.dirty <- struct{}{}:
	default:
	}
	if s.cfg.OnDirty != nil {
		s.cfg.OnDirty(cc.id)
	}
}

// renderPaneAdapter adapts a session.Pane to the render.Pane interface,
// bridging the pane.CellGrid and render.CellGrid types.
type renderPaneAdapter struct {
	p    session.Pane
	rect render.Rect
}

func (a *renderPaneAdapter) Bounds() render.Rect { return a.rect }

func (a *renderPaneAdapter) Snapshot() render.CellGrid {
	snap := a.p.Snapshot()
	cells := make([]render.Cell, len(snap.Cells))
	for i, c := range snap.Cells {
		cells[i] = render.Cell{Char: c.Char}
	}
	return render.CellGrid{Rows: snap.Rows, Cols: snap.Cols, Cells: cells}
}

// renderLoop runs as a per-client goroutine and sends a full-repaint
// MsgStdout to the client each time cc.dirty signals a redraw. It exits
// when cc.dirty is closed (on client disconnect).
func (s *srv) renderLoop(cc *clientConn) {
	defer s.wg.Done()
	for range cc.dirty {
		s.mu.Lock()
		client := cc.client
		if client.Session == nil || client.Session.Current == nil {
			s.mu.Unlock()
			continue
		}
		win := client.Session.Current.Window
		if win.Layout == nil || len(win.Panes) == 0 {
			s.mu.Unlock()
			continue
		}

		cols := client.Size.Cols
		rows := client.Size.Rows
		if cols <= 0 {
			cols = 80
		}
		if rows <= 0 {
			rows = 24
		}

		placements := make([]render.PanePlacement, 0, len(win.Panes))
		for id, p := range win.Panes {
			rect := win.Layout.Rect(id)
			if rect.Width == 0 || rect.Height == 0 {
				continue
			}
			placements = append(placements, render.PanePlacement{
				Pane: &renderPaneAdapter{p: p, rect: rect},
				Rect: rect,
			})
		}
		s.mu.Unlock()

		r := render.New(render.Config{Rows: rows, Cols: cols})
		grid := r.Compose(placements, nil)
		ansiBytes := render.EncodeANSI(grid)

		msg := proto.StdoutMsg{Data: ansiBytes}
		_ = proto.WriteMsg(cc.netConn, proto.MsgStdout, msg.Encode())
	}
}
