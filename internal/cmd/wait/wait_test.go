package wait_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/cmd"
	"github.com/dhamidi/dmux/internal/cmd/args"
	"github.com/dhamidi/dmux/internal/cmd/wait"
	"github.com/dhamidi/dmux/internal/options"
	"github.com/dhamidi/dmux/internal/proto"
	"github.com/dhamidi/dmux/internal/record"
)

// fakeItem satisfies cmd.Item with only the surface wait exercises:
// Context() feeds the subscription's parent context. Everything else
// returns zero values — calls to them from wait would indicate a
// scope regression and should surface via a nil-deref at test time.
type fakeItem struct {
	ctx context.Context
}

func (i *fakeItem) Context() context.Context         { return i.ctx }
func (*fakeItem) Shutdown(string)                    {}
func (*fakeItem) Client() cmd.Client                 { return nil }
func (*fakeItem) Sessions() cmd.SessionLookup        { return nil }
func (*fakeItem) SetAttachTarget(cmd.SessionRef)     {}
func (*fakeItem) SetDetach(proto.ExitReason, string) {}
func (*fakeItem) Options() *options.Options          { return nil }
func (*fakeItem) Clients() cmd.ClientManager         { return nil }
func (*fakeItem) CurrentSession() cmd.SessionRef     { return nil }
func (*fakeItem) SpawnWindow(cmd.SessionRef, string) (cmd.WindowRef, error) {
	return nil, nil
}
func (*fakeItem) AdvanceWindow(cmd.SessionRef, int) (cmd.WindowRef, error) {
	return nil, nil
}

func newFakeItem() *fakeItem {
	return &fakeItem{ctx: context.Background()}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func lookupWait(t *testing.T) cmd.Command {
	t.Helper()
	c, ok := cmd.Lookup(wait.Name)
	if !ok {
		t.Fatalf("%q not registered", wait.Name)
	}
	return c
}

// openRecorder opens a recorder for the duration of the test. Open
// serializes on a process-wide mutex, so tests that use it run
// sequentially by virtue of t.Cleanup enforcing Close before the
// next test's Open.
func openRecorder(t *testing.T) {
	t.Helper()
	if err := record.Open(record.Config{Logger: discardLogger()}); err != nil {
		t.Fatalf("record.Open: %v", err)
	}
	t.Cleanup(func() { _ = record.Close() })
}

func TestWaitMatchesEventName(t *testing.T) {
	openRecorder(t)
	c := lookupWait(t)
	item := newFakeItem()

	done := make(chan cmd.Result, 1)
	go func() {
		done <- c.Exec(item, []string{wait.Name, "pane.ready"})
	}()

	// Give the subscription a moment to install before emitting.
	time.Sleep(5 * time.Millisecond)
	record.Emit(context.Background(), "pane.ready", "pane", 1)

	select {
	case res := <-done:
		if !res.OK() {
			t.Fatalf("wait returned %v, want Ok", res.Error())
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("wait did not return in time")
	}
}

func TestWaitTextPredicateMatches(t *testing.T) {
	openRecorder(t)
	c := lookupWait(t)
	item := newFakeItem()

	done := make(chan cmd.Result, 1)
	go func() {
		done <- c.Exec(item, []string{wait.Name, "pty.output", "hello"})
	}()

	time.Sleep(5 * time.Millisecond)
	// Events whose text field does not contain the needle must be
	// filtered out; the wait should ignore this one and keep
	// waiting for the next.
	record.Emit(context.Background(), "pty.output", "text", "nope")
	record.Emit(context.Background(), "pty.output", "text", "hello world")

	select {
	case res := <-done:
		if !res.OK() {
			t.Fatalf("wait returned %v, want Ok", res.Error())
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("wait did not return in time")
	}
}

func TestWaitTextPredicateIgnoresMismatch(t *testing.T) {
	openRecorder(t)
	c := lookupWait(t)
	item := newFakeItem()

	// Short timeout so the mismatch path fails fast.
	done := make(chan cmd.Result, 1)
	go func() {
		done <- c.Exec(item, []string{wait.Name, "-T", "50ms", "pty.output", "hello"})
	}()

	time.Sleep(5 * time.Millisecond)
	record.Emit(context.Background(), "pty.output", "text", "nope")

	select {
	case res := <-done:
		if res.OK() {
			t.Fatalf("wait matched a non-containing event, want timeout")
		}
		if !errors.Is(res.Error(), wait.ErrTimeout) {
			t.Fatalf("error does not wrap ErrTimeout: %v", res.Error())
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("wait did not return in time")
	}
}

func TestWaitTimeoutReturnsErrTimeout(t *testing.T) {
	if d, ok := t.Deadline(); ok {
		if time.Until(d) < 500*time.Millisecond {
			t.Skip("not enough time left on deadline")
		}
	}
	openRecorder(t)
	c := lookupWait(t)
	item := newFakeItem()

	start := time.Now()
	res := c.Exec(item, []string{wait.Name, "-T", "50ms", "never.fires"})
	elapsed := time.Since(start)

	if res.OK() {
		t.Fatalf("wait for never-emitted event returned Ok")
	}
	if !errors.Is(res.Error(), wait.ErrTimeout) {
		t.Fatalf("error does not wrap ErrTimeout: %v", res.Error())
	}
	if elapsed > 300*time.Millisecond {
		t.Fatalf("wait with 50ms timeout took %s, want under 300ms", elapsed)
	}
}

func TestWaitMissingEventIsParseError(t *testing.T) {
	openRecorder(t)
	c := lookupWait(t)
	item := newFakeItem()

	res := c.Exec(item, []string{wait.Name})
	if res.OK() {
		t.Fatalf("wait without event returned Ok")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Name != "event" {
		t.Fatalf("ParseError.Name = %q, want %q", perr.Name, "event")
	}
}

func TestWaitBadDurationIsParseError(t *testing.T) {
	openRecorder(t)
	c := lookupWait(t)
	item := newFakeItem()

	res := c.Exec(item, []string{wait.Name, "foo", "-T", "bogus"})
	if res.OK() {
		t.Fatalf("wait with bogus duration returned Ok")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Name != "T" {
		t.Fatalf("ParseError.Name = %q, want %q", perr.Name, "T")
	}
	if perr.Phase != "flags" {
		t.Fatalf("ParseError.Phase = %q, want %q", perr.Phase, "flags")
	}
}

func TestWaitBadQuotedTextIsParseError(t *testing.T) {
	openRecorder(t)
	c := lookupWait(t)
	item := newFakeItem()

	res := c.Exec(item, []string{wait.Name, "foo", `\z`})
	if res.OK() {
		t.Fatalf("wait with malformed escape returned Ok")
	}
	var perr *args.ParseError
	if !errors.As(res.Error(), &perr) {
		t.Fatalf("error not *args.ParseError: %v", res.Error())
	}
	if perr.Name != "text" {
		t.Fatalf("ParseError.Name = %q, want %q", perr.Name, "text")
	}
}

func TestWaitDecodesGoEscapes(t *testing.T) {
	openRecorder(t)
	c := lookupWait(t)
	item := newFakeItem()

	done := make(chan cmd.Result, 1)
	go func() {
		// "\n" in argv is the literal two characters backslash-n;
		// strconv.Unquote decodes it to a newline, so the emitted
		// "line one\nline two" must match.
		done <- c.Exec(item, []string{wait.Name, "pty.output", `\n`})
	}()

	time.Sleep(5 * time.Millisecond)
	record.Emit(context.Background(), "pty.output", "text", "line one\nline two")

	select {
	case res := <-done:
		if !res.OK() {
			t.Fatalf("wait with \\n needle returned %v, want Ok", res.Error())
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("wait did not return in time")
	}
}

func TestWaitMissingTextFieldIsNotMatch(t *testing.T) {
	openRecorder(t)
	c := lookupWait(t)
	item := newFakeItem()

	done := make(chan cmd.Result, 1)
	go func() {
		done <- c.Exec(item, []string{wait.Name, "-T", "50ms", "pty.output", "hello"})
	}()

	time.Sleep(5 * time.Millisecond)
	// Event name matches but there is no `text` field at all.
	record.Emit(context.Background(), "pty.output", "bytes", 4)
	// And a wrong-type text field (not a string).
	record.Emit(context.Background(), "pty.output", "text", 123)

	select {
	case res := <-done:
		if res.OK() {
			t.Fatalf("wait matched without usable text field, want timeout")
		}
		if !errors.Is(res.Error(), wait.ErrTimeout) {
			t.Fatalf("error does not wrap ErrTimeout: %v", res.Error())
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("wait did not return in time")
	}
}
