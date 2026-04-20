package control

import "sync"

// EventBus is a thread-safe publish/subscribe hub that implements [EventSource].
// Multiple subscribers can register handlers; each published event is delivered
// to all currently registered handlers.
type EventBus struct {
	mu       sync.Mutex
	handlers map[int]func(Event)
	nextID   int
}

// NewEventBus returns an empty, ready-to-use EventBus.
func NewEventBus() *EventBus {
	return &EventBus{handlers: make(map[int]func(Event))}
}

// Subscribe registers handler to receive events and returns an unsubscribe
// function. Calling the returned function removes the handler; it is
// idempotent and safe to call from any goroutine.
func (b *EventBus) Subscribe(handler func(Event)) func() {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.handlers[id] = handler
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		delete(b.handlers, id)
		b.mu.Unlock()
	}
}

// Publish delivers e to all currently registered handlers. Handlers are
// invoked with the internal mutex released to avoid deadlocks.
func (b *EventBus) Publish(e Event) {
	b.mu.Lock()
	handlers := make([]func(Event), 0, len(b.handlers))
	for _, h := range b.handlers {
		handlers = append(handlers, h)
	}
	b.mu.Unlock()
	for _, h := range handlers {
		h(e)
	}
}
