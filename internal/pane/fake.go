package pane

import (
	"sync"

	"github.com/dhamidi/dmux/internal/keys"
)

// FakeTerminal is an in-memory [Terminal] for use in tests.
// It records all calls and returns configurable values without
// requiring a real terminal emulator library.
//
// FakeTerminal is safe for concurrent use.
type FakeTerminal struct {
	mu      sync.Mutex
	written []byte
	title   string
	grid    CellGrid
	resizes []ResizeCall
	closed  bool
}

// ResizeCall records a single call to Resize.
type ResizeCall struct {
	Cols int
	Rows int
}

// SetTitle configures the title returned by [FakeTerminal.Title].
func (f *FakeTerminal) SetTitle(title string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.title = title
}

// SetGrid configures the [CellGrid] returned by [FakeTerminal.Snapshot].
func (f *FakeTerminal) SetGrid(grid CellGrid) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.grid = grid
}

// Written returns a copy of all bytes fed to [FakeTerminal.Write].
func (f *FakeTerminal) Written() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := make([]byte, len(f.written))
	copy(b, f.written)
	return b
}

// ResizeCalls returns a copy of all resize calls in order.
func (f *FakeTerminal) ResizeCalls() []ResizeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ResizeCall, len(f.resizes))
	copy(out, f.resizes)
	return out
}

// Closed reports whether [FakeTerminal.Close] has been called.
func (f *FakeTerminal) Closed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func (f *FakeTerminal) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.written = append(f.written, p...)
	return len(p), nil
}

func (f *FakeTerminal) Resize(cols, rows int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resizes = append(f.resizes, ResizeCall{Cols: cols, Rows: rows})
	return nil
}

func (f *FakeTerminal) Title() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.title, nil
}

func (f *FakeTerminal) Snapshot() CellGrid {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.grid
}

func (f *FakeTerminal) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

// FakeKeyEncoder is an in-memory [KeyEncoder] for use in tests.
// It records every key passed to [FakeKeyEncoder.Encode] and returns
// the key's string representation as the encoded bytes.
//
// FakeKeyEncoder is safe for concurrent use.
type FakeKeyEncoder struct {
	mu      sync.Mutex
	encoded []keys.Key
	closed  bool
}

// Encoded returns a copy of all keys passed to [FakeKeyEncoder.Encode].
func (f *FakeKeyEncoder) Encoded() []keys.Key {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]keys.Key, len(f.encoded))
	copy(out, f.encoded)
	return out
}

// Closed reports whether [FakeKeyEncoder.Close] has been called.
func (f *FakeKeyEncoder) Closed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func (f *FakeKeyEncoder) Encode(key keys.Key) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.encoded = append(f.encoded, key)
	return []byte(key.String()), nil
}

func (f *FakeKeyEncoder) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

// FakeMouseEncoder is an in-memory [MouseEncoder] for use in tests.
// It records every event passed to [FakeMouseEncoder.Encode] and
// returns a fixed marker byte slice as the encoded output.
//
// FakeMouseEncoder is safe for concurrent use.
type FakeMouseEncoder struct {
	mu      sync.Mutex
	encoded []keys.MouseEvent
	closed  bool
}

// Encoded returns a copy of all events passed to [FakeMouseEncoder.Encode].
func (f *FakeMouseEncoder) Encoded() []keys.MouseEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]keys.MouseEvent, len(f.encoded))
	copy(out, f.encoded)
	return out
}

// Closed reports whether [FakeMouseEncoder.Close] has been called.
func (f *FakeMouseEncoder) Closed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func (f *FakeMouseEncoder) Encode(ev keys.MouseEvent) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.encoded = append(f.encoded, ev)
	return []byte("mouse"), nil
}

func (f *FakeMouseEncoder) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}
