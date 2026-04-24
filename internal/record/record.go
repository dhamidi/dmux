package record

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// Level controls which emission call sites are live. Normal is always
// on; Debug adds the high-volume call sites listed in docs/testing.md
// (`vt.feed`, `socket.read`, `loop.iter`). In production the level is
// always Normal — SetLevel is a dmuxtest-only API.
type Level int32

const (
	// LevelNormal emits the closed event vocabulary (~30 events).
	LevelNormal Level = iota

	// LevelDebug additionally emits per-byte, per-iteration events.
	LevelDebug
)

// Event is one observable state transition. At is the wall-clock time
// the producer called Emit; Name is the dotted event name from the
// closed vocabulary (see docs/testing.md); Fields carries the keyed
// payload. Fields is nil when Emit was called with no kv pairs.
type Event struct {
	At     time.Time
	Name   string
	Fields map[string]any
}

// Filter selects which events a subscriber receives. A nil Filter is
// equivalent to "accept every event".
type Filter func(Event) bool

// Config parameterizes the recorder at Open time.
//
//   - Logger is the slog destination for production file sink; nil
//     falls back to slog.Default().
//   - Capacity is the size of the internal event buffer. Events that
//     arrive when the buffer is full are counted via Dropped() and
//     discarded; zero or negative selects the default (1024).
type Config struct {
	Logger   *slog.Logger
	Capacity int
}

const defaultCapacity = 1024

// ErrAlreadyOpen is returned by Open when a recorder is already live.
// Callers must Close first.
var ErrAlreadyOpen = errors.New("record: recorder already open")

// Recorder is the process-wide event sink. One instance is live
// between Open and Close; the package-level Emit / Subscribe /
// Unsubscribe / SetLevel functions operate on whichever recorder is
// currently live. The type is exported so test code under the
// dmuxtest build tag can reason about it, but normal callers route
// through the package-level functions.
type Recorder struct {
	in     chan Event
	logger *slog.Logger

	level   atomic.Int32
	dropped atomic.Uint64
	closed  atomic.Bool

	subMu sync.Mutex
	subs  []*subscription

	done chan struct{}
	wg   sync.WaitGroup
}

type subscription struct {
	ch     chan Event
	filter Filter
	closed atomic.Bool
}

var (
	globalMu sync.Mutex
	global   *Recorder
)

// Open installs a new recorder as the process-wide sink. Returns
// ErrAlreadyOpen if a previous Open has not been matched by Close.
// The returned recorder starts a consumer goroutine that drains the
// event buffer until Close.
func Open(cfg Config) error {
	globalMu.Lock()
	defer globalMu.Unlock()
	if global != nil {
		return ErrAlreadyOpen
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	capacity := cfg.Capacity
	if capacity <= 0 {
		capacity = defaultCapacity
	}
	r := &Recorder{
		in:     make(chan Event, capacity),
		logger: logger,
		done:   make(chan struct{}),
	}
	r.wg.Add(1)
	go r.run()
	global = r
	return nil
}

// Close tears down the current recorder. Pending events in the
// internal buffer are discarded; all subscriber channels are closed.
// Safe to call when no recorder is open.
func Close() error {
	globalMu.Lock()
	r := global
	global = nil
	globalMu.Unlock()
	if r == nil {
		return nil
	}
	return r.close()
}

// Emit records a Normal-level event. Safe to call when no recorder is
// open — the call is a no-op. kv is interpreted as alternating
// string keys and arbitrary values; mismatches mirror slog's
// "!BADKEY" convention.
func Emit(ctx context.Context, name string, kv ...any) {
	_ = ctx
	r := currentRecorder()
	if r == nil {
		return
	}
	r.send(Event{At: time.Now(), Name: name, Fields: fieldsFromKV(kv)})
}

// EmitDebug records a Debug-level event. When the current recorder's
// level is Normal (the production default), EmitDebug is cheap — it
// returns without allocating the event. Call sites for high-volume
// diagnostic events should use EmitDebug instead of Emit so production
// does not pay their cost.
func EmitDebug(ctx context.Context, name string, kv ...any) {
	_ = ctx
	r := currentRecorder()
	if r == nil || Level(r.level.Load()) < LevelDebug {
		return
	}
	r.send(Event{At: time.Now(), Name: name, Fields: fieldsFromKV(kv)})
}

// Dropped reports the cumulative number of events discarded because
// the internal buffer or a subscriber channel was full. Returns 0
// when no recorder is open. Intended for scenario assertions;
// operators see the counter in the log.
func Dropped() uint64 {
	r := currentRecorder()
	if r == nil {
		return 0
	}
	return r.dropped.Load()
}

// CurrentLevel returns the current recorder's verbosity, or
// LevelNormal when no recorder is open. Production code guarding
// Debug emissions calls this rather than SetLevel's companion Level()
// (which lives in the dmuxtest build tag).
func CurrentLevel() Level {
	r := currentRecorder()
	if r == nil {
		return LevelNormal
	}
	return Level(r.level.Load())
}

func currentRecorder() *Recorder {
	globalMu.Lock()
	defer globalMu.Unlock()
	return global
}

func (r *Recorder) send(ev Event) {
	if r.closed.Load() {
		return
	}
	select {
	case r.in <- ev:
	default:
		r.dropped.Add(1)
	}
}

func (r *Recorder) run() {
	defer r.wg.Done()
	for {
		select {
		case ev := <-r.in:
			r.emit(ev)
		case <-r.done:
			r.drain()
			return
		}
	}
}

// drain processes any events still buffered at close. Emissions in
// flight when close() was called — send() passed the closed check but
// had not yet landed in the channel — race with drain's default arm;
// those losses are accepted per docs/testing.md ("events not lost at
// production rates"). Sequential Emit → Close, the common case in
// both production and tests, is drained completely.
func (r *Recorder) drain() {
	for {
		select {
		case ev := <-r.in:
			r.emit(ev)
		default:
			return
		}
	}
}

func (r *Recorder) emit(ev Event) {
	r.log(ev)
	r.fanout(ev)
}

func (r *Recorder) log(ev Event) {
	if r.logger == nil {
		return
	}
	attrs := make([]slog.Attr, 0, len(ev.Fields))
	for k, v := range ev.Fields {
		attrs = append(attrs, slog.Any(k, v))
	}
	r.logger.LogAttrs(context.Background(), slog.LevelInfo, ev.Name, attrs...)
}

func (r *Recorder) fanout(ev Event) {
	r.subMu.Lock()
	subs := append([]*subscription(nil), r.subs...)
	r.subMu.Unlock()
	for _, s := range subs {
		if s.closed.Load() {
			continue
		}
		if s.filter != nil && !s.filter(ev) {
			continue
		}
		select {
		case s.ch <- ev:
		default:
			r.dropped.Add(1)
		}
	}
}

func (r *Recorder) close() error {
	if !r.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(r.done)
	r.wg.Wait()
	r.subMu.Lock()
	subs := r.subs
	r.subs = nil
	r.subMu.Unlock()
	for _, s := range subs {
		if s.closed.CompareAndSwap(false, true) {
			close(s.ch)
		}
	}
	return nil
}

func fieldsFromKV(kv []any) map[string]any {
	if len(kv) == 0 {
		return nil
	}
	m := make(map[string]any, (len(kv)+1)/2)
	for i := 0; i < len(kv); {
		k, ok := kv[i].(string)
		if !ok {
			m[fmt.Sprintf("!BADKEY#%d", i)] = kv[i]
			i++
			continue
		}
		if i+1 >= len(kv) {
			m["!MISSING"] = k
			break
		}
		m[k] = kv[i+1]
		i += 2
	}
	return m
}
