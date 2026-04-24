// Package record is dmux's structured event recorder.
//
// The recorder emits named events for every server state transition
// a test or operator might want to observe: session created, pane
// spawned, command executed, frame sent, pty output sampled, mode
// changed, client attached. In production, events go to the log
// file. In tests, they feed a matcher engine that drives scenarios.
//
// Same events either way. There is no separate "test recorder" — the
// production recorder IS the test oracle. This keeps the tested code
// path identical to the shipping code path and means user bug reports
// can be replayed against dmux as scenarios.
//
// # Event model
//
// Every event has a stable name (dotted, e.g. `pane.ready`),
// a timestamp, and a typed payload of named fields. Names come from
// a closed vocabulary — see docs/testing.md for the full list. Fields
// carry stable ID sigils: `$N` session, `@N` window, `%N` pane,
// `#N` client, `!N` command item.
//
// Events are NOT instructions. The recorder observes the server; it
// doesn't let tests inject behaviour. Scenarios drive state change
// by running real commands (`new-session`, `send-keys`, `at`) and
// wait for the resulting events, not by calling recorder methods.
//
// # Emission
//
//	record.Emit(ctx, "pane.ready", "pane", p.ID)
//	record.Emit(ctx, "pty.output", "pane", p.ID, "text", sample)
//	record.Emit(ctx, "cmd.result", "item", item.ID, "status", "ok")
//
// Emit is cheap in the hot path: allocates an Event, pushes it on a
// buffered channel, returns. A consumer goroutine drains the channel
// to the log file and (under test) to subscribed matchers. If the
// channel fills (producer faster than consumer, shouldn't happen at
// normal recorder level), events are dropped with a `record.dropped`
// counter increment. Under `recorder set-level debug`, the
// channel capacity is higher and dropping is treated as a scenario
// failure.
//
// # Levels
//
// Two levels:
//
//	Normal  default. The ~30 events listed in docs/testing.md.
//	Debug   adds per-byte, per-loop-iteration events for fine-grained
//	        scenario assertions. Opt-in via recorder set-level.
//
// In production, always Normal. The log file grows predictably —
// roughly one event per user action plus a handful per second under
// load.
//
// # Subscription
//
// Subscribe/Unsubscribe are always available. Scenarios consume them
// for assertions; production hooks and plugins consume them to
// observe the event stream in-process.
//
//	ch := record.Subscribe(ctx, filter)
//	defer record.Unsubscribe(ch)
//
// Subscribers receive every matching event from the moment of
// subscription until Unsubscribe or ctx.Done. Multiple subscribers
// are independent; one slow subscriber does not block others (its
// own channel fills and its events drop into the Dropped counter).
//
// The filter is a predicate function; scenarios compose these via
// the matcher library in internal/dmuxtest, and hooks can use any
// predicate they like.
//
// # Event sampling for high-volume payloads
//
// `pty.output` carries text so tests can wait on pane content. The
// full byte stream is too much; we sample. Default: last 256 bytes
// of the most recent pty read. A test waiting for "hello" in a pane
// that's emitting log spam may need to broaden the window; the
// `wait output` test command handles this by accumulating samples
// over multiple events.
//
// # Production file sink
//
// In production, events go to the slog-based log file (see
// internal/log) as slog.Record values. Each event's fields become
// slog.Attr pairs. Operators can tail the log and see the event
// stream in human-readable form. Third-party tooling can consume
// the structured output.
//
// # Interface
//
//	Emit(ctx context.Context, name string, kv ...any)
//	EmitDebug(ctx context.Context, name string, kv ...any)
//	Open(cfg Config) error
//	Close() error
//	Subscribe(ctx context.Context, filter Filter) <-chan Event
//	Unsubscribe(ch <-chan Event)
//	SetLevel(Level)
//	CurrentLevel() Level
//	Dropped() uint64
//
//	type Event struct {
//	    At     time.Time
//	    Name   string
//	    Fields map[string]any       // typed, stable across versions
//	}
//
//	type Filter func(Event) bool
//
// # Guarantees
//
//   - Event order is preserved per source goroutine. Two events from
//     the same goroutine appear in emission order to every subscriber.
//   - Event order across goroutines is not guaranteed. A test that
//     depends on "A happened before B" where A and B come from
//     different goroutines must use events that have a happens-before
//     relationship in the real system — e.g. `cmd.exec` before
//     `cmd.result` for the same item.
//   - Events are not lost under Normal level at production rates.
//     Under load, Debug level may drop; Normal never does.
//
// # What's not in the recorder
//
// The recorder is for observable events, not for assertions or
// inspection. If a scenario needs to check "is the alt-screen
// enabled right now", that's `assert alt-screen -t %0`, which reads
// vt state directly through cmd.Host — not a recorder query.
//
// # Scope boundary
//
//   - No serialization format besides slog. If remote ingestion lands
//     later, it wraps slog's Handler.
//   - No retention policy. Production log rotation is the only
//     mechanism; tests consume live and discard.
//   - No replay into a stopped server. Scenarios replay by rerunning
//     the command sequence, not by feeding events back.
//
// # Corresponding tmux code
//
// No equivalent in tmux. tmux's log.c is the closest analogue — it
// also writes debug lines to a file — but its logs are unstructured
// strings, not a test-usable event stream.
package record
