package session

import "fmt"

// Set creates or replaces a named buffer, making it the top of the stack when
// new. If name is empty, a name is auto-generated as "bufferN" where N is the
// current stack length.
func (bs *BufferStack) Set(name, data string) {
	if name == "" {
		name = fmt.Sprintf("buffer%d", len(bs.buffers))
	}
	for _, b := range bs.buffers {
		if b.Name == name {
			b.Data = []byte(data)
			return
		}
	}
	bs.Push(name, []byte(data))
}

// GetNamed returns the buffer with the given name, or nil/false if not found.
func (bs *BufferStack) GetNamed(name string) (*PasteBuffer, bool) {
	for _, b := range bs.buffers {
		if b.Name == name {
			return b, true
		}
	}
	return nil, false
}

// DeleteNamed removes the buffer with the given name.
// It returns true if a buffer was found and removed.
func (bs *BufferStack) DeleteNamed(name string) bool {
	for i, b := range bs.buffers {
		if b.Name == name {
			bs.Delete(i)
			return true
		}
	}
	return false
}

// List returns a snapshot of all buffers in stack order (index 0 is the top).
func (bs *BufferStack) List() []*PasteBuffer {
	out := make([]*PasteBuffer, len(bs.buffers))
	copy(out, bs.buffers)
	return out
}
