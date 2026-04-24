//go:build dmuxtest

package record_test

import (
	"context"
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

func TestSetLevelPromotesDebug(t *testing.T) {
	if err := record.Open(record.Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = record.Close() })

	record.SetLevel(record.LevelDebug)
	if record.CurrentLevel() != record.LevelDebug {
		t.Fatalf("CurrentLevel=%v, want LevelDebug", record.CurrentLevel())
	}

	ch := record.Subscribe(context.Background(), nil)
	record.EmitDebug(context.Background(), "vt.feed", "pane", 0)

	got := collect(t, ch, 1, time.Second)
	if got[0].Name != "vt.feed" {
		t.Fatalf("EmitDebug did not deliver at LevelDebug: %v", names(got))
	}
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
