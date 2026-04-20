package session

import "github.com/dhamidi/dmux/internal/layout"

// SessionID uniquely identifies a session within a Server.
type SessionID string

// WindowID uniquely identifies a window.
type WindowID string

// ClientID uniquely identifies a connected client.
type ClientID string

// PaneID is an alias for [layout.LeafID], identifying a pane slot in a Window.
// Using an alias (not a new type) means layout functions such as
// [layout.Tree.Split] and [layout.Tree.Close] accept PaneID values directly.
type PaneID = layout.LeafID

// Size describes the visible dimensions of a client terminal in character cells.
type Size struct {
	Cols int
	Rows int
}

// FeatureSet is a bitmask of optional capabilities a client advertises.
type FeatureSet uint32

const (
	FeatureColour256 FeatureSet = 1 << iota // 256-colour palette
	FeatureColour16M                         // 24-bit true colour
	FeatureMouseSGR                          // SGR mouse protocol
	FeatureOverlap                           // overlapping-window support
)

// Environ is a captured process environment: an immutable name→value snapshot.
type Environ map[string]string

// Clone returns a deep copy of e.
func (e Environ) Clone() Environ {
	out := make(Environ, len(e))
	for k, v := range e {
		out[k] = v
	}
	return out
}

// Pane is the interface that the session package requires of a running pane.
// The full implementation lives in [internal/pane]; session holds panes by
// this interface so it stays independent of the concrete pane type.
type Pane interface {
	// Title returns the pane's current window title (from OSC 2 or similar).
	Title() string
	// Resize updates the pane's visible dimensions.
	// It returns an error if the underlying PTY or terminal cannot be resized.
	Resize(cols, rows int) error
	// Close shuts down the pane's child process and frees its resources.
	Close() error
	// CaptureContent returns the visible terminal content as plain text.
	// If history is true, scrollback content is included when available.
	CaptureContent(history bool) ([]byte, error)
	// Respawn kills the current child process and starts a fresh one using
	// the given shell (falls back to $SHELL or /bin/sh if empty).
	Respawn(shell string) error
}

// Overlay is the interface that the session package requires of a client
// overlay or interactive mode. Concrete types live in [internal/modes];
// session holds overlays by this interface to avoid a Tier-3 import.
type Overlay interface {
	// OverlayName returns a stable identifier used for logging and debug
	// output (e.g. "copy-mode", "tree-mode").
	OverlayName() string
}

// PasteBuffer is a named clipboard buffer owned by the Server.
type PasteBuffer struct {
	Name string
	Data []byte
}

// BufferStack is an ordered stack of paste buffers.
// Index 0 is the most-recently added ("top") buffer.
type BufferStack struct {
	buffers []*PasteBuffer
}

// Push prepends a new buffer, making it the top of the stack.
func (bs *BufferStack) Push(name string, data []byte) {
	cp := make([]byte, len(data))
	copy(cp, data)
	bs.buffers = append([]*PasteBuffer{{Name: name, Data: cp}}, bs.buffers...)
}

// Pop removes and returns the top buffer, or nil if the stack is empty.
func (bs *BufferStack) Pop() *PasteBuffer {
	if len(bs.buffers) == 0 {
		return nil
	}
	top := bs.buffers[0]
	bs.buffers = bs.buffers[1:]
	return top
}

// Top returns the top buffer without removing it, or nil if the stack is empty.
func (bs *BufferStack) Top() *PasteBuffer {
	if len(bs.buffers) == 0 {
		return nil
	}
	return bs.buffers[0]
}

// Len returns the number of buffers in the stack.
func (bs *BufferStack) Len() int {
	return len(bs.buffers)
}

// Get returns the buffer at index i, or nil if i is out of range.
func (bs *BufferStack) Get(i int) *PasteBuffer {
	if i < 0 || i >= len(bs.buffers) {
		return nil
	}
	return bs.buffers[i]
}

// Delete removes the buffer at index i. It is a no-op if i is out of range.
func (bs *BufferStack) Delete(i int) {
	if i < 0 || i >= len(bs.buffers) {
		return
	}
	bs.buffers = append(bs.buffers[:i], bs.buffers[i+1:]...)
}

// HookTable stores named lists of callbacks to invoke on lifecycle events.
type HookTable struct {
	hooks map[string][]func()
}

// Register appends fn to the list of callbacks for name.
func (ht *HookTable) Register(name string, fn func()) {
	if ht.hooks == nil {
		ht.hooks = make(map[string][]func())
	}
	ht.hooks[name] = append(ht.hooks[name], fn)
}

// Run invokes all callbacks registered under name in registration order.
func (ht *HookTable) Run(name string) {
	for _, fn := range ht.hooks[name] {
		fn()
	}
}
