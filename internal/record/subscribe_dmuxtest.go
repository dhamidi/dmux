//go:build dmuxtest

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
// Multiple subscribers are independent. A slow subscriber's channel
// fills and causes its own events to drop (counted via Dropped())
// without affecting other subscribers.
//
// Subscribe is only compiled under the dmuxtest build tag.
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
	r.subs = append(r.subs, sub)
	r.subMu.Unlock()
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
//
// Unsubscribe is only compiled under the dmuxtest build tag.
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

// SetLevel updates the current recorder's verbosity. Scenario files
// invoke this through the test-set-recorder-level command. When no
// recorder is open, SetLevel is a no-op.
//
// SetLevel is only compiled under the dmuxtest build tag.
func SetLevel(lv Level) {
	r := currentRecorder()
	if r == nil {
		return
	}
	r.level.Store(int32(lv))
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
