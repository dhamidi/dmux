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
	"github.com/dhamidi/dmux/internal/control"
	"github.com/dhamidi/dmux/internal/format"
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/layout"
	"github.com/dhamidi/dmux/internal/modes"
	copymode "github.com/dhamidi/dmux/internal/modes/copy"
	"github.com/dhamidi/dmux/internal/osinfo"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/pty"
	"github.com/dhamidi/dmux/internal/render"
	"github.com/dhamidi/dmux/internal/session"
	"github.com/dhamidi/dmux/internal/shell"
	"github.com/dhamidi/dmux/internal/status"
	"github.com/dhamidi/dmux/internal/style"
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

	// OverlayPusher, if non-nil, is called after a client completes the
	// identify handshake. It returns a ClientOverlay to push onto that
	// client's overlay stack, or nil to push nothing. Intended for tests
	// that need to inject an overlay without going through a command.
	OverlayPusher func(clientID session.ClientID) modes.ClientOverlay

	// State, if non-nil, is used as the initial server state instead of
	// creating a new empty Server. Primarily useful in tests that need
	// to pre-populate sessions, windows, or panes.
	State *session.Server

	// ForegroundCommand, if non-nil, is called on each status tick to resolve
	// the name of the foreground process for a given shell PID. It is used by
	// the automatic-rename feature. Defaults to osinfo.Default().ForegroundCommand
	// (ignoring errors) when nil. Tests may inject a stub.
	ForegroundCommand func(pid int) string
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
	cfg             Config
	state           *session.Server
	store           command.Server
	mutator         command.Mutator
	queue           *command.Queue
	log             *log.Logger
	mu              sync.Mutex
	conns           map[session.ClientID]*clientConn
	decoders        map[session.ClientID]*clientDecoder
	prevFrames      map[session.ClientID]render.CellGrid
	nextID          uint64
	done            chan struct{}
	once            sync.Once // ensures done is closed at most once
	wg              sync.WaitGroup
	events          *control.EventBus
	controlSessions map[session.ClientID]*control.ControlSession
}

// clientConn is the server-side view of one connected client.
type clientConn struct {
	id          session.ClientID
	netConn     net.Conn
	client      *session.Client
	dirty       chan struct{}     // buffered(1); written to when a redraw is needed
	dragBorder  *layout.BorderID // non-nil while a border drag is in progress
	controlMode bool             // set when attach-session -C or new-session -C runs

	// overlays is a stack of client-scoped overlays (menus, popups, prompts).
	// The last element is on top and receives key events first.
	// Access is protected by srv.mu.
	overlays []modes.ClientOverlay

	// paneOverlays holds per-pane modes (copy-mode, clock-mode) keyed by
	// pane ID. Access is protected by srv.mu.
	paneOverlays map[session.PaneID]modes.PaneMode
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
	if cfg.ForegroundCommand == nil {
		osinfoClient := osinfo.Default()
		cfg.ForegroundCommand = func(pid int) string {
			name, _ := osinfoClient.ForegroundCommand(pid)
			return name
		}
	}

	s := &srv{
		cfg:             cfg,
		log:             log.New(cfg.Log, "server: ", 0),
		conns:           make(map[session.ClientID]*clientConn),
		decoders:        make(map[session.ClientID]*clientDecoder),
		prevFrames:      make(map[session.ClientID]render.CellGrid),
		done:            make(chan struct{}),
		events:          control.NewEventBus(),
		controlSessions: make(map[session.ClientID]*control.ControlSession),
	}
	if cfg.State != nil {
		s.state = cfg.State
	} else {
		s.state = session.NewServer()
	}
	s.store = newServerStore(s.state)
	s.queue = command.NewQueue()
	sm := newServerMutator(s.state, s.store, s.queue, s.triggerDone,
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
		func(paneID int) { s.startPaneWatcher(paneID) },
		s.events,
	)
	sm.clearPrevFrame = func(id session.ClientID) {
		s.mu.Lock()
		delete(s.prevFrames, id)
		s.mu.Unlock()
	}
	sm.pushOverlayFn = func(id session.ClientID, ov modes.ClientOverlay) {
		s.pushOverlay(id, ov)
	}
	sm.popOverlayFn = func(id session.ClientID) {
		s.popOverlay(id)
	}
	sm.pushPaneOverlayFn = func(id session.ClientID, paneID session.PaneID, mode modes.PaneMode) {
		s.pushPaneOverlay(id, paneID, mode)
	}
	sm.popPaneOverlayFn = func(id session.ClientID, paneID session.PaneID) {
		s.popPaneOverlay(id, paneID)
	}
	s.mutator = sm

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
	go s.tickLoop()

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

// pushOverlay appends ov to the top of the overlay stack for the named client
// and schedules a redraw. It is safe to call from any goroutine.
func (s *srv) pushOverlay(clientID session.ClientID, ov modes.ClientOverlay) {
	s.mu.Lock()
	cc, ok := s.conns[clientID]
	if ok {
		cc.overlays = append(cc.overlays, ov)
	}
	s.mu.Unlock()
	if ok {
		s.markDirty(cc)
	}
}

// popOverlay removes the topmost overlay from the named client's stack,
// calls Close on it, and schedules a redraw. It is safe to call from any goroutine.
func (s *srv) popOverlay(clientID session.ClientID) {
	s.mu.Lock()
	cc, ok := s.conns[clientID]
	var popped modes.ClientOverlay
	if ok && len(cc.overlays) > 0 {
		n := len(cc.overlays)
		popped = cc.overlays[n-1]
		cc.overlays = cc.overlays[:n-1]
	}
	s.mu.Unlock()
	if popped != nil {
		popped.Close()
		s.mu.Lock()
		cc, ok = s.conns[clientID]
		s.mu.Unlock()
		if ok {
			s.markDirty(cc)
		}
	}
}

// pushPaneOverlay registers mode as the active PaneMode for the given pane on
// the given client, replacing any previous mode. It schedules a redraw.
func (s *srv) pushPaneOverlay(clientID session.ClientID, paneID session.PaneID, mode modes.PaneMode) {
	s.mu.Lock()
	cc, ok := s.conns[clientID]
	if ok {
		if cc.paneOverlays == nil {
			cc.paneOverlays = make(map[session.PaneID]modes.PaneMode)
		}
		cc.paneOverlays[paneID] = mode
	}
	s.mu.Unlock()
	if ok {
		s.markDirty(cc)
	}
}

// popPaneOverlay removes the PaneMode for the given pane on the given client,
// calls Close on it, and schedules a redraw.
func (s *srv) popPaneOverlay(clientID session.ClientID, paneID session.PaneID) {
	s.mu.Lock()
	cc, ok := s.conns[clientID]
	var popped modes.PaneMode
	if ok && cc.paneOverlays != nil {
		if mode, exists := cc.paneOverlays[paneID]; exists {
			popped = mode
			delete(cc.paneOverlays, paneID)
		}
	}
	s.mu.Unlock()
	if popped != nil {
		popped.Close()
		s.mu.Lock()
		cc, ok = s.conns[clientID]
		s.mu.Unlock()
		if ok {
			s.markDirty(cc)
		}
	}
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

	// If an OverlayPusher is configured (e.g. in tests), push the returned
	// overlay onto the client's stack immediately after registration.
	if s.cfg.OverlayPusher != nil {
		if ov := s.cfg.OverlayPusher(client.ID); ov != nil {
			s.pushOverlay(client.ID, ov)
		}
	}

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

	// Send SGR mouse enable sequences if the session has mouse turned on.
	s.mu.Lock()
	if cc.client.Session != nil {
		if mouseOn, _ := cc.client.Session.Options.GetBool("mouse"); mouseOn {
			out := proto.StdoutMsg{Data: []byte("\x1b[?1006h\x1b[?1000h")}
			_ = proto.WriteMsg(cc.netConn, proto.MsgStdout, out.Encode())
		}
	}
	s.mu.Unlock()

	defer func() {
		// Send SGR mouse disable sequences before detaching.
		out := proto.StdoutMsg{Data: []byte("\x1b[?1000l\x1b[?1006l")}
		_ = proto.WriteMsg(cc.netConn, proto.MsgStdout, out.Encode())
		s.mu.Lock()
		delete(s.conns, client.ID)
		delete(s.decoders, client.ID)
		s.state.DetachClient(client.ID)
		close(cc.dirty)
		s.mu.Unlock()
		s.log.Printf("client %s detached", client.ID)
		s.mutator.RunHook("client-detached")
	}()

	// Step 4: start per-client render goroutine, then enter message loop.
	s.wg.Add(1)
	go s.renderLoop(cc)

	s.clientLoop(cc)

	// If the client requested control mode, run the control mode loop.
	if cc.controlMode {
		s.controlLoop(cc)
	}
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
			if result.ControlMode {
				cc.controlMode = true
				return
			}
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

				if k.Code == keys.CodeMouse {
					s.handleMouseEvent(cc, k.Mouse)
					continue
				}

				// Route through the topmost focus-capturing overlay, if any.
				s.mu.Lock()
				var captureOverlay modes.ClientOverlay
				if n := len(cc.overlays); n > 0 {
					top := cc.overlays[n-1]
					if top.CaptureFocus() {
						captureOverlay = top
					}
				}
				s.mu.Unlock()

				if captureOverlay != nil {
					outcome := captureOverlay.Key(k)
					skipNormal := true
					switch outcome.Kind {
					case modes.KindCloseMode:
						s.mu.Lock()
						if n := len(cc.overlays); n > 0 {
							popped := cc.overlays[n-1]
							cc.overlays = cc.overlays[:n-1]
							s.mu.Unlock()
							popped.Close()
						} else {
							s.mu.Unlock()
						}
					case modes.KindPassthrough:
						skipNormal = false
					}
					if skipNormal {
						continue
					}
				}

				// Route key to pane mode (copy-mode, clock-mode) when active.
				// Pane modes take priority over the key table and raw pane input.
				s.mu.Lock()
				var activePaneMode modes.PaneMode
				var activePaneModeID session.PaneID
				if cc.paneOverlays != nil && cc.client.Session != nil && cc.client.Session.Current != nil {
					win := cc.client.Session.Current.Window
					if mode, ok := cc.paneOverlays[win.Active]; ok {
						activePaneMode = mode
						activePaneModeID = win.Active
					}
				}
				s.mu.Unlock()

				if activePaneMode != nil {
					outcome := activePaneMode.Key(k)
					switch outcome.Kind {
					case modes.KindCloseMode:
						s.popPaneOverlay(cc.id, activePaneModeID)
					case modes.KindCommand:
						if copyCmd, ok := outcome.Cmd.(copymode.CopyCommand); ok {
							s.mu.Lock()
							s.state.Buffers.Push("", []byte(copyCmd.Text))
							s.mu.Unlock()
							s.popPaneOverlay(cc.id, activePaneModeID)
						}
					}
					s.markDirty(cc)
					continue
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
				var syncPanes []session.Pane
				if !isBound && cc.client.Session != nil && cc.client.Session.Current != nil {
					win := cc.client.Session.Current.Window
					if pane, ok := win.Panes[win.Active]; ok {
						activePane = pane
					}
					if activePane != nil {
						if on, _ := win.Options.GetBool("synchronize-panes"); on {
							for pid, p := range win.Panes {
								if pid == win.Active {
									continue
								}
								syncPanes = append(syncPanes, p)
							}
						}
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
					for _, p := range syncPanes {
						_ = p.Write(rawBytes)
					}
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

// markAllDirty schedules a redraw for every connected client.
func (s *srv) markAllDirty() {
	for _, cc := range s.conns {
		s.markDirty(cc)
	}
}

// tickLoop fires once per second to perform periodic tasks such as
// automatic window renaming. It exits when s.done is closed.
func (s *srv) tickLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			s.autoRenameWindows()
			s.monitorWindows()
			s.mu.Unlock()
		}
	}
}

// monitorWindows checks monitor-activity, monitor-silence, and monitor-bell
// window options and alerts clients when conditions are met.
// Must be called with s.mu held.
func (s *srv) monitorWindows() {
	now := s.cfg.Now()
	for _, sess := range s.state.Sessions {
		for _, wl := range sess.Windows {
			win := wl.Window
			for _, p := range win.Panes {
				if on, _ := win.Options.GetBool("monitor-activity"); on {
					if p.LastOutputAt().After(win.LastMonitorCheck) {
						win.ActivityFlag = true
						s.sendBellToClients(sess)
					}
				}
				silenceSec, _ := win.Options.GetInt("monitor-silence")
				if silenceSec > 0 && !p.LastOutputAt().IsZero() {
					if now.Sub(p.LastOutputAt()) > time.Duration(silenceSec)*time.Second {
						s.sendBellToClients(sess)
					}
				}
				if on, _ := win.Options.GetBool("monitor-bell"); on {
					if p.ConsumeBell() {
						win.ActivityFlag = true
						s.sendBellToClients(sess)
					}
				}
			}
			win.LastMonitorCheck = now
		}
	}
}

// sendBellToClients sends a BEL character to all clients attached to sess.
func (s *srv) sendBellToClients(sess *session.Session) {
	for _, cc := range s.conns {
		if cc.client.Session != nil && cc.client.Session.ID == sess.ID {
			_ = proto.WriteMsg(cc.netConn, proto.MsgStdout, []byte("\x07"))
		}
	}
}

// autoRenameWindows updates window names based on the foreground process of
// the active pane when the automatic-rename window option is on.
// Must be called with s.mu held.
func (s *srv) autoRenameWindows() {
	for _, sess := range s.state.Sessions {
		for _, wl := range sess.Windows {
			win := wl.Window
			enabled, _ := win.Options.GetBool("automatic-rename")
			if !enabled {
				continue
			}
			activePane, ok := win.Panes[win.Active]
			if !ok {
				continue
			}
			pid := activePane.ShellPID()
			if pid == 0 {
				continue
			}
			name := s.cfg.ForegroundCommand(pid)
			if name != "" && name != win.Name {
				win.Name = name
				s.markAllDirty()
			}
		}
	}
}

// startPaneWatcher starts a goroutine that fires the pane-died hook when the
// pane's child process exits (ShellPID transitions from non-zero to zero).
// If remain-on-exit is set on the window, the hook is not fired and the pane
// is not killed automatically.
func (s *srv) startPaneWatcher(paneID int) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		wasAlive := false
		for {
			select {
			case <-s.done:
				return
			case <-ticker.C:
			}
			s.mu.Lock()
			var p session.Pane
			var remainOnExit bool
			for _, sess := range s.state.Sessions {
				for _, wl := range sess.Windows {
					win := wl.Window
					if pane, ok := win.Panes[session.PaneID(paneID)]; ok {
						p = pane
						remainOnExit, _ = win.Options.GetBool("remain-on-exit")
					}
				}
			}
			s.mu.Unlock()
			if p == nil {
				// Pane was removed from state; stop watching.
				return
			}
			pid := p.ShellPID()
			if pid > 0 {
				wasAlive = true
			} else if wasAlive && !remainOnExit {
				// Process was alive before and is now dead.
				s.mutator.RunHook("pane-died")
				_ = s.mutator.KillPane(paneID)
				return
			}
		}
	}()
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
		cells[i] = render.Cell{
			Char:  c.Char,
			Fg:    c.Fg,
			Bg:    c.Bg,
			Attrs: c.Attrs,
			FgR:   c.FgR,
			FgG:   c.FgG,
			FgB:   c.FgB,
			BgR:   c.BgR,
			BgG:   c.BgG,
			BgB:   c.BgB,
		}
	}
	return render.CellGrid{Rows: snap.Rows, Cols: snap.Cols, Cells: cells}
}

// paneModeAdapter implements render.Pane by rendering a PaneMode onto a
// temporary canvas and returning the result as a CellGrid.
type paneModeAdapter struct {
	mode modes.PaneMode
	rect render.Rect
}

func (a *paneModeAdapter) Bounds() render.Rect { return a.rect }

func (a *paneModeAdapter) Snapshot() render.CellGrid {
	rows := a.rect.Height
	cols := a.rect.Width
	if rows <= 0 || cols <= 0 {
		return render.CellGrid{Rows: rows, Cols: cols}
	}
	canvas := &gridCanvas{
		rows:  rows,
		cols:  cols,
		cells: make([]modes.Cell, rows*cols),
	}
	a.mode.Render(canvas)
	renderCells := make([]render.Cell, len(canvas.cells))
	for i, c := range canvas.cells {
		renderCells[i] = render.Cell{
			Char:  c.Char,
			Fg:    render.Color(c.Fg),
			Bg:    render.Color(c.Bg),
			Attrs: uint16(c.Attrs),
			FgR:   c.FgR,
			FgG:   c.FgG,
			FgB:   c.FgB,
			BgR:   c.BgR,
			BgG:   c.BgG,
			BgB:   c.BgB,
		}
	}
	return render.CellGrid{Rows: rows, Cols: cols, Cells: renderCells}
}

// gridCanvas implements modes.Canvas over a flat []modes.Cell slice.
type gridCanvas struct {
	rows, cols int
	cells      []modes.Cell
}

func (c *gridCanvas) Size() modes.Size { return modes.Size{Rows: c.rows, Cols: c.cols} }

func (c *gridCanvas) Set(col, row int, cell modes.Cell) {
	if col < 0 || col >= c.cols || row < 0 || row >= c.rows {
		return
	}
	c.cells[row*c.cols+col] = cell
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

		syncPanes, _ := win.Options.GetBool("synchronize-panes")
		borderLines, _ := win.Options.GetString("pane-border-lines")
		borderStatus, _ := win.Options.GetString("pane-border-status")
		borderFormat, _ := win.Options.GetString("pane-border-format")
		placements := make([]render.PanePlacement, 0, len(win.Panes))
		for id, p := range win.Panes {
			rect := win.Layout.Rect(id)
			if rect.Width == 0 || rect.Height == 0 {
				continue
			}
			var paneFace render.Pane
			if cc.paneOverlays != nil {
				if mode, hasMode := cc.paneOverlays[id]; hasMode {
					paneFace = &paneModeAdapter{mode: mode, rect: rect}
				}
			}
			if paneFace == nil {
				paneFace = &renderPaneAdapter{p: p, rect: rect}
			}
			placements = append(placements, render.PanePlacement{
				Pane:              paneFace,
				Rect:              rect,
				SynchronizedPanes: syncPanes,
				PaneIndex:         int(id),
			})
		}

		// Capture status-related values and overlay stack while holding the lock.
		statusOn, _ := client.Session.Options.GetString("status")
		sessionName := client.Session.Name
		windowName := win.Name
		sessOpts := client.Session.Options
		overlayStack := make([]modes.ClientOverlay, len(cc.overlays))
		copy(overlayStack, cc.overlays)
		s.mu.Unlock()

		// Build the status line when the status option is "on".
		var statusLine render.StatusLine
		statusRows := 0
		if statusOn == "on" {
			fmtCtx := format.MapContext{
				"session_name": sessionName,
				"window_name":  windowName,
			}
			sl := status.New(
				&formatExpanderAdapter{e: format.New(nil)},
				fmtCtx,
				&status.StoreOptions{Store: sessOpts},
			)
			statusLine = &renderStatusAdapter{sl: sl}
			statusRows = 1
		}

		r := render.New(render.Config{
			Rows:       rows,
			Cols:       cols,
			Status:     statusLine,
			StatusRows: statusRows,
			Theme: render.Theme{
				BorderLines:      borderLines,
				PaneBorderStatus: borderStatus,
				PaneBorderFormat: borderFormat,
			},
		})
		renderOverlays := make([]render.Overlay, len(overlayStack))
		for i, ov := range overlayStack {
			renderOverlays[i] = &overlayRenderAdapter{ov: ov}
		}
		grid := r.Compose(placements, renderOverlays)

		// Look up the previous frame for this client and store the current one.
		s.mu.Lock()
		prevGrid, hasPrev := s.prevFrames[cc.id]
		s.prevFrames[cc.id] = grid
		s.mu.Unlock()

		// On the first render, or when the terminal was resized, fall back to a
		// full repaint. Otherwise emit only the cells that changed.
		var ansiBytes []byte
		if hasPrev && prevGrid.Rows == grid.Rows && prevGrid.Cols == grid.Cols {
			ansiBytes = render.EncodeDiffANSI(prevGrid, grid)
		} else {
			ansiBytes = render.EncodeANSI(grid)
		}

		msg := proto.StdoutMsg{Data: ansiBytes}
		_ = proto.WriteMsg(cc.netConn, proto.MsgStdout, msg.Encode())
	}
}

// mouseEventRaw reconstructs an SGR mouse escape sequence from a MouseEvent.
// This is used to forward mouse events to panes as raw bytes.
func mouseEventRaw(m keys.MouseEvent) []byte {
	var bits int
	switch m.Button {
	case keys.MouseLeft:
		bits = 0
	case keys.MouseMiddle:
		bits = 1
	case keys.MouseRight:
		bits = 2
	case keys.MouseWheelUp:
		bits = 64
	case keys.MouseWheelDown:
		bits = 65
	default:
		bits = 3
	}
	if m.Action == keys.MouseMotion {
		bits |= 32
	}
	final := 'M'
	if m.Action == keys.MouseRelease {
		final = 'm'
	}
	return []byte(fmt.Sprintf("\x1b[<%d;%d;%d%c", bits, m.Col+1, m.Row+1, final))
}

// handleMouseEvent routes a decoded mouse event to the appropriate handler
// based on the session's mouse option and the event type.
func (s *srv) handleMouseEvent(cc *clientConn, m keys.MouseEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess := cc.client.Session
	if sess == nil {
		return
	}

	// Guard: when the mouse option is off, forward raw bytes to the active pane.
	mouseOn, _ := sess.Options.GetBool("mouse")
	if !mouseOn {
		if sess.Current != nil {
			win := sess.Current.Window
			if win != nil {
				if p, ok := win.Panes[win.Active]; ok {
					_ = p.Write(mouseEventRaw(m))
				}
			}
		}
		return
	}

	if sess.Current == nil {
		return
	}
	win := sess.Current.Window
	if win == nil {
		return
	}

	switch {
	case m.Button == keys.MouseLeft && m.Action == keys.MousePress:
		// Click to focus: find the pane under the cursor and select it.
		paneID, ok := layout.PaneAt(win.Layout, m.Col, m.Row)
		if ok && paneID != win.Active {
			_ = s.mutator.SelectPane(string(sess.ID), string(win.ID), int(paneID))
		}

	case m.Button == keys.MouseLeft && m.Action == keys.MouseMotion:
		// Border drag: detect a border and record it; attempt resize.
		borderID, ok := layout.BorderAt(win.Layout, m.Col, m.Row)
		if ok {
			cc.dragBorder = borderID
			_ = s.mutator.ResizePane(int(borderID.PaneID), "R", 1)
		}

	case m.Button == keys.MouseWheelUp || m.Button == keys.MouseWheelDown:
		// Wheel scroll: forward raw bytes to the active pane.
		if p, ok := win.Panes[win.Active]; ok {
			_ = p.Write(mouseEventRaw(m))
		}
	}
}

// controlWriter wraps a clientConn and sends MsgStdout frames.
type controlWriter struct {
	cc *clientConn
}

func (cw *controlWriter) Write(p []byte) (int, error) {
	msg := proto.StdoutMsg{Data: p}
	if err := proto.WriteMsg(cw.cc.netConn, proto.MsgStdout, msg.Encode()); err != nil {
		return 0, err
	}
	return len(p), nil
}

// controlLoop runs the tmux control-mode protocol for a client that requested
// control mode via attach-session -C or new-session -C.
func (s *srv) controlLoop(cc *clientConn) {
	s.mu.Lock()
	clientView, _ := s.store.GetClient(string(cc.client.ID))
	s.mu.Unlock()

	cs := control.NewControlSession(
		&controlWriter{cc: cc},
		s.store,
		s.mutator,
		s.queue,
		clientView,
	)
	defer cs.Close()

	// Subscribe to the server's global event bus and forward events to the
	// per-session bus so the Writer serialises them to the client.
	unsub := s.events.Subscribe(func(e control.Event) {
		cs.Bus().Publish(e)
	})
	defer unsub()

	// Register in the server's control session map.
	s.mu.Lock()
	s.controlSessions[cc.id] = cs
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.controlSessions, cc.id)
		s.mu.Unlock()
	}()

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
				cs.ResizeClient(int(m.Width), int(m.Height))
				s.mu.Lock()
				cc.client.Size = session.Size{Cols: int(m.Width), Rows: int(m.Height)}
				s.mu.Unlock()
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
			clientView, _ = s.store.GetClient(string(cc.client.ID))
			s.mu.Unlock()
			cs.UpdateClient(clientView)
			cs.HandleCommand(cm.Argv)
		case proto.MsgShutdown:
			s.triggerDone()
			return
		}
	}
}

// formatExpanderAdapter adapts *format.Expander to the status.Expander interface.
// format.Expander.Expand returns (string, error); status.Expander.Expand returns string.
type formatExpanderAdapter struct {
	e *format.Expander
}

func (a *formatExpanderAdapter) Expand(template string, ctx status.Context) string {
	var fmtCtx format.Context
	if fc, ok := ctx.(format.Context); ok {
		fmtCtx = fc
	} else {
		fmtCtx = &statusContextWrapper{ctx: ctx}
	}
	result, _ := a.e.Expand(template, fmtCtx)
	return result
}

// statusContextWrapper adapts a status.Context to format.Context by adding
// a no-op Children implementation.
type statusContextWrapper struct {
	ctx status.Context
}

func (w *statusContextWrapper) Lookup(key string) (string, bool) {
	return w.ctx.Lookup(key)
}

func (w *statusContextWrapper) Children(_ string) []format.Context {
	return nil
}

// overlayRenderAdapter adapts a modes.ClientOverlay to the render.Overlay
// interface, bridging the modes.Cell and render.Cell type mismatch.
type overlayRenderAdapter struct {
	ov modes.ClientOverlay
}

func (a *overlayRenderAdapter) Rect() render.Rect { return a.ov.Rect() }

func (a *overlayRenderAdapter) Render(dst []render.Cell) {
	modeCells := make([]modes.Cell, len(dst))
	a.ov.Render(modeCells)
	for i, c := range modeCells {
		dst[i] = render.Cell{
			Char:  c.Char,
			Fg:    render.Color(c.Fg),
			Bg:    render.Color(c.Bg),
			Attrs: uint16(c.Attrs),
			FgR:   c.FgR,
			FgG:   c.FgG,
			FgB:   c.FgB,
			BgR:   c.BgR,
			BgG:   c.BgG,
			BgB:   c.BgB,
		}
	}
}

// renderStatusAdapter adapts *status.StatusLine to render.StatusLine,
// converting status.Cell values (carrying style.Style) to render.Cell values.
type renderStatusAdapter struct {
	sl *status.StatusLine
}

func (a *renderStatusAdapter) Render(width int) []render.Cell {
	statusCells := a.sl.Render(width)
	if statusCells == nil {
		return nil
	}
	cells := make([]render.Cell, len(statusCells))
	for i, sc := range statusCells {
		cells[i] = render.Cell{
			Char:  sc.Char,
			Fg:    sc.Style.Fg,
			FgR:   sc.Style.FgR,
			FgG:   sc.Style.FgG,
			FgB:   sc.Style.FgB,
			Bg:    sc.Style.Bg,
			BgR:   sc.Style.BgR,
			BgG:   sc.Style.BgG,
			BgB:   sc.Style.BgB,
			Attrs: statusAttrsToRenderAttrs(sc.Style.Attrs),
		}
	}
	return cells
}

// statusAttrsToRenderAttrs converts style package attribute bits to render
// package attribute bits. The two packages define the same attributes but
// assign them to different bit positions.
func statusAttrsToRenderAttrs(sAttrs uint16) uint16 {
	var r uint16
	if sAttrs&style.AttrBold != 0 {
		r |= render.AttrBold
	}
	if sAttrs&style.AttrItalics != 0 {
		r |= render.AttrItalics
	}
	if sAttrs&style.AttrUnderscore != 0 {
		r |= render.AttrUnderline
	}
	if sAttrs&style.AttrDoubleUnderscore != 0 {
		r |= render.AttrDoubleUnderline
	}
	if sAttrs&style.AttrCurlyUnderscore != 0 {
		r |= render.AttrCurlyUnderline
	}
	if sAttrs&style.AttrOverline != 0 {
		r |= render.AttrOverline
	}
	if sAttrs&style.AttrBlink != 0 {
		r |= render.AttrBlink
	}
	if sAttrs&style.AttrReverse != 0 {
		r |= render.AttrReverse
	}
	if sAttrs&style.AttrDim != 0 {
		r |= render.AttrDim
	}
	if sAttrs&style.AttrStrikethrough != 0 {
		r |= render.AttrStrikethrough
	}
	return r
}
