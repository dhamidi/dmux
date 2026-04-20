package control

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/dhamidi/dmux/internal/command"
)

// ControlSession manages the tmux control-mode protocol for a single client.
// It wraps an [io.Writer] and dispatches incoming commands through the normal
// command infrastructure, emitting %begin/%end blocks for each command and
// serialising asynchronous server events as bare protocol lines.
//
// Construct a ControlSession with [NewControlSession]. Call [ControlSession.Bus]
// to obtain the [EventBus] the server should use to push state-change events.
// Call [ControlSession.Close] when the client disconnects.
type ControlSession struct {
	out io.Writer  // raw destination; all writes go through mu
	mu  sync.Mutex // serialises writes to out
	bus *EventBus
	evw *Writer // event serialiser; writes via lockedWriter → out

	seqMu sync.Mutex
	seq   int

	now     func() int64
	store   command.Server
	mutator command.Mutator
	queue   *command.Queue
	client  command.ClientView

	// Width and Height are the client's declared terminal dimensions.
	// They are updated by ResizeClient and used for window sizing.
	Width  int
	Height int
}

// lockedWriter serialises writes to an io.Writer using a shared mutex.
type lockedWriter struct {
	w  io.Writer
	mu *sync.Mutex
}

func (lw *lockedWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.w.Write(p)
}

// NewControlSession creates a ControlSession that writes protocol lines to out.
// Event lines emitted through the returned session's [EventBus] are serialised
// by a [Writer] that shares out's write lock, preventing interleaving with
// command-output blocks.
func NewControlSession(
	out io.Writer,
	store command.Server,
	mut command.Mutator,
	queue *command.Queue,
	client command.ClientView,
) *ControlSession {
	cs := &ControlSession{
		out:     out,
		bus:     NewEventBus(),
		now:     func() int64 { return time.Now().Unix() },
		store:   store,
		mutator: mut,
		queue:   queue,
		client:  client,
	}
	cs.evw = NewWriter(&lockedWriter{w: out, mu: &cs.mu}, cs.bus)
	return cs
}

// Bus returns the EventBus the server uses to publish state-change events.
// Published events are serialised as protocol lines between command blocks.
func (cs *ControlSession) Bus() *EventBus { return cs.bus }

// Close unsubscribes from the EventBus and stops event delivery.
func (cs *ControlSession) Close() { cs.evw.Close() }

// ResizeClient updates the control client's declared terminal dimensions.
// This does not affect visual rendering (control mode has no TUI); it sets
// the size used for window sizing calculations.
func (cs *ControlSession) ResizeClient(width, height int) {
	cs.Width = width
	cs.Height = height
}

// UpdateClient updates the client view used for target resolution.
func (cs *ControlSession) UpdateClient(client command.ClientView) {
	cs.client = client
}

// nextSeq returns and increments the command sequence number.
func (cs *ControlSession) nextSeq() int {
	cs.seqMu.Lock()
	defer cs.seqMu.Unlock()
	n := cs.seq
	cs.seq++
	return n
}

// HandleCommand dispatches argv through the command registry and writes a
// %begin / %end block (or %begin / %error on failure) to the output.
// The entire block is written atomically under the internal write lock so
// that asynchronous event lines cannot interleave within it.
func (cs *ControlSession) HandleCommand(argv []string) {
	if len(argv) == 0 {
		return
	}
	ts := cs.now()
	seq := cs.nextSeq()

	// Dispatch the command. This may trigger synchronous event publications
	// which will be serialised before the %begin block.
	result := command.Dispatch(argv[0], argv[1:], cs.store, cs.client, cs.queue, cs.mutator)
	if cs.queue != nil {
		cs.queue.Drain()
	}

	// Write the %begin…%end (or %error) block atomically.
	cs.mu.Lock()
	defer cs.mu.Unlock()

	fmt.Fprintf(cs.out, "%%begin %d %d 0\n", ts, seq)
	if result.Err != nil {
		fmt.Fprintf(cs.out, "%s\n", result.Err.Error())
		fmt.Fprintf(cs.out, "%%error %d %d 0\n", ts, seq)
	} else {
		if result.Output != "" {
			out := result.Output
			if !strings.HasSuffix(out, "\n") {
				out += "\n"
			}
			io.WriteString(cs.out, out)
		}
		fmt.Fprintf(cs.out, "%%end %d %d 0\n", ts, seq)
	}
}
