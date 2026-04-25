package record

import (
	"context"
)

const subscriberCapacity = 256

// Subscribe registers filter against the currently open recorder and
// returns a channel that receives every matching event from the
// moment of subscription until ctx.Done() or Unsubscribe. A nil
// filter matches every event. When no recorder is open, Subscribe
// returns a closed channel so select loops exit promptly.
//
// When the recorder was opened with Config.ReplayBufferSize > 0,
// Subscribe first replays the ring of recent events into the new
// channel (filter applied) before returning. This lets scenarios
// that subscribe after a triggering command finished still observe
// the events that command emitted. The snapshot is captured under
// the same lock that registers the subscriber, so no event is both
// missing from the replay and missing from live fanout. A live
// event may interleave with replay delivery — the consumer
// goroutine's fanout writes can land in the channel before the
// Subscribe goroutine finishes pushing the snapshot — so callers
// must not assume strict snapshot-then-live ordering.
//
// Multiple subscribers are independent. A slow subscriber's channel
// fills and causes its own events to drop (counted via Dropped())
// without affecting other subscribers.
//
// Subscribe is always available: scenario runners consume it for
// assertions, and production hooks/plugins consume it to observe the
// server's event stream in-process.
func Subscribe(ctx context.Context, filter Filter) <-chan Event {
	r := currentRecorder()
	if r == nil {
		ch := make(chan Event)
		close(ch)
		return ch
	}
	sub := &subscription{
		ch:     make(chan Event, subscriberCapacity),
		filter: filter,
	}
	r.subMu.Lock()
	replay := r.ringSnapshotLocked()
	r.subs = append(r.subs, sub)
	r.subMu.Unlock()
	for _, ev := range replay {
		if filter != nil && !filter(ev) {
			continue
		}
		select {
		case sub.ch <- ev:
		default:
			r.dropped.Add(1)
		}
	}
	if ctx != nil {
		go func() {
			<-ctx.Done()
			r.removeSubscription(sub)
		}()
	}
	return sub.ch
}

// Unsubscribe removes ch from the current recorder's fanout list and
// closes it. Safe to call more than once with the same channel; safe
// when no recorder is open.
func Unsubscribe(ch <-chan Event) {
	r := currentRecorder()
	if r == nil {
		return
	}
	r.subMu.Lock()
	defer r.subMu.Unlock()
	kept := r.subs[:0]
	for _, s := range r.subs {
		if (<-chan Event)(s.ch) == ch {
			if s.closed.CompareAndSwap(false, true) {
				close(s.ch)
			}
			continue
		}
		kept = append(kept, s)
	}
	r.subs = kept
}

func (r *Recorder) removeSubscription(s *subscription) {
	r.subMu.Lock()
	defer r.subMu.Unlock()
	kept := r.subs[:0]
	for _, existing := range r.subs {
		if existing == s {
			continue
		}
		kept = append(kept, existing)
	}
	r.subs = kept
	if s.closed.CompareAndSwap(false, true) {
		close(s.ch)
	}
}
