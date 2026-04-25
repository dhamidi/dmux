package record_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/record"
)

func TestSubscribeReceivesEmissions(t *testing.T) {
	if err := record.Open(record.Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = record.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := record.Subscribe(ctx, nil)

	record.Emit(context.Background(), "pane.ready", "pane", 1)
	record.Emit(context.Background(), "pane.resize", "pane", 1, "cols", 80, "rows", 24)

	got := collect(t, ch, 2, time.Second)
	if got[0].Name != "pane.ready" || got[1].Name != "pane.resize" {
		t.Fatalf("events out of order: %v", names(got))
	}
	if got[1].Fields["cols"] != 80 {
		t.Fatalf("cols field lost: %v", got[1].Fields)
	}
}

func TestSubscribeFilterExcludes(t *testing.T) {
	if err := record.Open(record.Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = record.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	filter := func(e record.Event) bool { return e.Name == "pane.ready" }
	ch := record.Subscribe(ctx, filter)

	record.Emit(context.Background(), "pane.resize")
	record.Emit(context.Background(), "pane.ready")
	record.Emit(context.Background(), "pane.resize")

	got := collect(t, ch, 1, time.Second)
	if got[0].Name != "pane.ready" {
		t.Fatalf("filter did not exclude: got %v", names(got))
	}
	select {
	case ev, ok := <-ch:
		if ok {
			t.Fatalf("unexpected extra event %q", ev.Name)
		}
	case <-time.After(50 * time.Millisecond):
	}
}

func TestMultipleSubscribersAreIndependent(t *testing.T) {
	if err := record.Open(record.Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = record.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := record.Subscribe(ctx, nil)
	b := record.Subscribe(ctx, nil)

	record.Emit(context.Background(), "cmd.exec", "item", 1)

	gotA := collect(t, a, 1, time.Second)
	gotB := collect(t, b, 1, time.Second)
	if gotA[0].Name != "cmd.exec" || gotB[0].Name != "cmd.exec" {
		t.Fatalf("subscribers missed event: A=%v B=%v", names(gotA), names(gotB))
	}
}

func TestUnsubscribeClosesChannel(t *testing.T) {
	if err := record.Open(record.Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = record.Close() })

	ch := record.Subscribe(context.Background(), nil)
	record.Unsubscribe(ch)

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("channel delivered event after Unsubscribe")
		}
	case <-time.After(time.Second):
		t.Fatalf("channel not closed after Unsubscribe")
	}

	record.Unsubscribe(ch)
}

func TestContextCancelUnsubscribes(t *testing.T) {
	if err := record.Open(record.Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = record.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	ch := record.Subscribe(ctx, nil)
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("channel delivered after ctx cancel")
		}
	case <-time.After(time.Second):
		t.Fatalf("channel not closed after ctx cancel")
	}
}

func TestSubscribeBeforeOpenReturnsClosedChannel(t *testing.T) {
	ch := record.Subscribe(context.Background(), nil)
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("channel is not closed")
		}
	default:
		t.Fatalf("channel not ready when no recorder is open")
	}
}

func TestCloseClosesAllSubscribers(t *testing.T) {
	if err := record.Open(record.Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	ch := record.Subscribe(context.Background(), nil)
	if err := record.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("subscriber not closed after Close")
		}
	case <-time.After(time.Second):
		t.Fatalf("subscriber not closed after Close")
	}
}

// drainEmissions issues a sentinel emit and waits for it to surface
// on a live subscriber. By the time the sentinel arrives, every event
// emitted before it has already been processed by the consumer
// goroutine and appended to the ring. Returns once the sentinel is
// observed; fails the test on timeout.
func drainEmissions(t *testing.T, sentinel string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := record.Subscribe(ctx, func(e record.Event) bool { return e.Name == sentinel })
	record.Emit(context.Background(), sentinel)
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("sentinel %q did not arrive within 1s", sentinel)
	}
}

func TestSubscribeReplaysRing(t *testing.T) {
	if err := record.Open(record.Config{
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		ReplayBufferSize: 4,
	}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = record.Close() })

	record.Emit(context.Background(), "a")
	record.Emit(context.Background(), "b")
	record.Emit(context.Background(), "c")
	drainEmissions(t, "drain.replay")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	filter := func(e record.Event) bool { return e.Name != "drain.replay" }
	ch := record.Subscribe(ctx, filter)
	got := collect(t, ch, 3, time.Second)
	if got[0].Name != "a" || got[1].Name != "b" || got[2].Name != "c" {
		t.Fatalf("replay order wrong: %v", names(got))
	}
}

func TestSubscribeReplayRespectsFilter(t *testing.T) {
	if err := record.Open(record.Config{
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		ReplayBufferSize: 8,
	}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = record.Close() })

	for _, n := range []string{"a", "b", "a", "b", "a"} {
		record.Emit(context.Background(), n)
	}
	drainEmissions(t, "drain.filter")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	filter := func(e record.Event) bool { return e.Name == "a" }
	ch := record.Subscribe(ctx, filter)
	got := collect(t, ch, 3, time.Second)
	for i, e := range got {
		if e.Name != "a" {
			t.Fatalf("filter failed at %d: %q", i, e.Name)
		}
	}
}

func TestRingEvictsOldest(t *testing.T) {
	if err := record.Open(record.Config{
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		ReplayBufferSize: 2,
	}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = record.Close() })

	record.Emit(context.Background(), "1")
	record.Emit(context.Background(), "2")
	record.Emit(context.Background(), "3")
	drainEmissions(t, "drain.evict")

	// Ring cap is 2. The drain sentinel itself was the latest write,
	// so it evicted "2" — ring now holds [3, drain.evict]. Filter the
	// sentinel out and assert the survivor is "3".
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	filter := func(e record.Event) bool { return e.Name != "drain.evict" }
	ch := record.Subscribe(ctx, filter)
	got := collect(t, ch, 1, time.Second)
	if got[0].Name != "3" {
		t.Fatalf("eviction wrong: %v", names(got))
	}
}

func TestSubscribeZeroBufferIsLiveOnly(t *testing.T) {
	if err := record.Open(record.Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = record.Close() })

	record.Emit(context.Background(), "old")
	// Give the consumer a moment to process the pre-Subscribe event so
	// it can't be confused with a live delivery to the new subscriber.
	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := record.Subscribe(ctx, nil)
	record.Emit(context.Background(), "new")

	got := collect(t, ch, 1, time.Second)
	if got[0].Name != "new" {
		t.Fatalf("expected live event, got %q", got[0].Name)
	}
	select {
	case ev, ok := <-ch:
		if ok {
			t.Fatalf("unexpected extra event %q (replay should be off)", ev.Name)
		}
	case <-time.After(50 * time.Millisecond):
	}
}

func TestOpenNegativeReplayBufferSizeRejected(t *testing.T) {
	err := record.Open(record.Config{ReplayBufferSize: -1})
	if !errors.Is(err, record.ErrInvalidReplayBufferSize) {
		t.Fatalf("expected ErrInvalidReplayBufferSize, got %v", err)
	}
	if err := record.Open(record.Config{ReplayBufferSize: 0}); err != nil {
		t.Fatalf("Open with 0 should succeed, got %v", err)
	}
	_ = record.Close()
	if err := record.Open(record.Config{ReplayBufferSize: 4}); err != nil {
		t.Fatalf("Open with positive should succeed, got %v", err)
	}
	_ = record.Close()
}

func collect(t *testing.T, ch <-chan record.Event, n int, timeout time.Duration) []record.Event {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var out []record.Event
	for len(out) < n {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("channel closed after %d/%d events", len(out), n)
			}
			out = append(out, ev)
		case <-deadline.C:
			t.Fatalf("timeout after %v with %d/%d events", timeout, len(out), n)
		}
	}
	return out
}

func names(evs []record.Event) []string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.Name
	}
	return out
}
