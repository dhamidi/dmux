package command

import (
	"io"
	"log"
)

// Item is a unit of work in the Queue. Exactly one of Run or Callback must be
// non-nil.
type Item struct {
	// Name is a label used in log output. It may be empty.
	Name string
	// Run is a zero-argument function executed synchronously by Drain.
	// Mutually exclusive with Callback.
	Run func()
	// Callback, when non-nil, causes Drain to pause the queue after invoking
	// it. The queue delivers a resume func as its argument; the caller stores
	// that func and calls it when the blocking operation (e.g. a command-prompt
	// confirmation) completes. No further items are processed until resume is
	// called.
	Callback func(resume func())
}

// Queue is an ordered, async command queue. Items are processed one at a time;
// a Callback item suspends processing until its resume func is called.
//
// Queue has no goroutines of its own. The server event loop drives it by
// calling Drain on each iteration. All logging is injectable via SetLogger;
// the default logger discards output so no hidden writes to os.Stderr occur.
type Queue struct {
	items  []Item
	paused bool
	log    *log.Logger
}

// NewQueue returns an empty Queue that discards all log output.
func NewQueue() *Queue {
	return &Queue{log: log.New(io.Discard, "", 0)}
}

// SetLogger replaces the logger used for queue trace messages.
// Pass w = nil to revert to the discard logger.
func (q *Queue) SetLogger(w io.Writer, prefix string, flags int) {
	if w == nil {
		w = io.Discard
	}
	q.log = log.New(w, prefix, flags)
}

// Enqueue appends item to the tail of the queue. It is safe to call from
// within a running Item.Run (the item is added after the current one and
// processed on the next Drain call, not recursively).
func (q *Queue) Enqueue(item Item) {
	q.items = append(q.items, item)
}

// EnqueueFunc is a convenience wrapper that enqueues a plain function.
func (q *Queue) EnqueueFunc(name string, fn func()) {
	q.Enqueue(Item{Name: name, Run: fn})
}

// Len returns the number of items waiting (not yet processed).
func (q *Queue) Len() int { return len(q.items) }

// IsPaused reports whether the queue is suspended waiting for a Callback to
// call its resume function.
func (q *Queue) IsPaused() bool { return q.paused }

// Drain processes items from the front of the queue until it is empty or a
// Callback item suspends it. It returns the number of items processed.
//
// Drain must not be called from within an Item.Run function; doing so has
// undefined behaviour (items enqueued by Run are visible to the outer Drain
// call).
func (q *Queue) Drain() int {
	processed := 0
	for !q.paused && len(q.items) > 0 {
		item := q.items[0]
		q.items = q.items[1:]
		processed++

		if item.Run != nil {
			q.log.Printf("queue: run %q", item.Name)
			item.Run()
		} else if item.Callback != nil {
			q.log.Printf("queue: pause for callback %q", item.Name)
			q.paused = true
			item.Callback(func() {
				q.log.Printf("queue: resume after callback %q", item.Name)
				q.paused = false
			})
		}
	}
	return processed
}
