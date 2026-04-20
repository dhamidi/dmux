package session

import "sync"

// namedChannel pairs a channel with a flag indicating it has been signalled
// (closed). Using a dedicated struct avoids double-close panics.
type namedChannel struct {
	ch     chan struct{}
	closed bool
}

// ChannelTable manages named synchronisation channels for wait-for / signal.
// It is safe for concurrent use.
type ChannelTable struct {
	mu       sync.Mutex
	channels map[string]*namedChannel
}

func (t *ChannelTable) getOrCreate(name string) *namedChannel {
	if t.channels == nil {
		t.channels = make(map[string]*namedChannel)
	}
	nc, ok := t.channels[name]
	if !ok {
		nc = &namedChannel{ch: make(chan struct{})}
		t.channels[name] = nc
	}
	return nc
}

// Wait blocks until Signal is called with the same name. If Signal was
// already called, Wait returns immediately.
func (t *ChannelTable) Wait(name string) error {
	t.mu.Lock()
	nc := t.getOrCreate(name)
	t.mu.Unlock()
	<-nc.ch
	return nil
}

// Signal unblocks all current and future waiters for name by closing the
// channel. Calling Signal more than once for the same name is a no-op.
func (t *ChannelTable) Signal(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	nc := t.getOrCreate(name)
	if !nc.closed {
		close(nc.ch)
		nc.closed = true
	}
}
