package pty

import (
	"bytes"
	"io"
	"sync"
)

// FakePTY is an in-memory PTY for use in tests.
// It requires no OS resources and implements the PTY interface.
//
// Bytes written by a caller (simulating keyboard input) are captured
// and can be retrieved with Input.  Bytes intended to be "read back"
// as child output must be pre-loaded with InjectOutput; subsequent
// Read calls drain from that buffer.
//
// FakePTY is safe for concurrent use.
type FakePTY struct {
	mu      sync.Mutex
	output  bytes.Buffer // bytes returned by Read; pre-load with InjectOutput
	input   bytes.Buffer // bytes captured from Write calls
	closed  bool

	// Resizes records each Resize call in the order it was received.
	Resizes []Size

	// FakePID is returned by Pid(). Set this to simulate a running process.
	FakePID int
}

// InjectOutput queues data to be returned by subsequent Read calls,
// simulating output produced by the child process.
func (f *FakePTY) InjectOutput(data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.output.Write(data)
}

// Read returns bytes previously queued with InjectOutput.
// It returns io.EOF once the fake PTY is closed and the buffer is empty.
func (f *FakePTY) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.output.Len() == 0 && f.closed {
		return 0, io.EOF
	}
	return f.output.Read(p)
}

// Write captures p as simulated keyboard input.
// It returns io.ErrClosedPipe after Close has been called.
func (f *FakePTY) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, io.ErrClosedPipe
	}
	return f.input.Write(p)
}

// Resize records the resize operation.
// It returns io.ErrClosedPipe after Close has been called.
func (f *FakePTY) Resize(rows, cols int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return io.ErrClosedPipe
	}
	f.Resizes = append(f.Resizes, Size{Rows: rows, Cols: cols})
	return nil
}

// Close marks the FakePTY as closed. Subsequent writes and resizes
// return an error; reads drain the remaining output buffer then return io.EOF.
func (f *FakePTY) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// Pid returns the fake PID configured via FakePID.
func (f *FakePTY) Pid() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.FakePID
}

// Input returns a copy of all bytes passed to Write so far.
func (f *FakePTY) Input() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := make([]byte, f.input.Len())
	copy(b, f.input.Bytes())
	return b
}
