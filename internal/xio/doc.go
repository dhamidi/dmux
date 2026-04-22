// Package xio provides length-prefixed frame I/O for dmux.
//
// It is shared by internal/client and internal/server, sitting on top
// of any io.Reader / io.Writer pair (typically a net.Conn returned by
// internal/socket).
//
// # Interface
//
//	type FrameReader interface {
//	    ReadFrame() (proto.Frame, error)
//	}
//
//	type FrameWriter interface {
//	    WriteFrame(proto.Frame) error
//	}
//
//	func NewReader(r io.Reader) FrameReader
//	func NewWriter(w io.Writer) FrameWriter
//
// A Frame is the on-wire representation defined in internal/proto.
// This package does not interpret frame contents; that is proto's job.
//
// # Concurrency
//
// A single FrameReader and a single FrameWriter per connection.
// Callers serialize writes. Reads from a FrameReader are sequential by
// construction; a reader is owned by exactly one goroutine.
//
// # Backpressure
//
// WriteFrame is synchronous. A writer that cannot keep up causes the
// owning goroutine to block; the server uses this as natural
// backpressure for Output frames on slow clients.
package xio
