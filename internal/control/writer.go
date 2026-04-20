package control

import (
	"encoding/base64"
	"fmt"
	"io"
)

// EventSource is the interface through which the Writer subscribes to
// server events. It decouples the control package from any concrete
// server or session type.
//
// Subscribe registers handler to be called for each event. It returns an
// unsubscribe function; calling that function removes the handler so that
// no further events are delivered. Unsubscribe is idempotent.
type EventSource interface {
	Subscribe(handler func(event Event)) (unsubscribe func())
}

// Writer serialises control-mode events to an [io.Writer] using the
// line-based text protocol described in the package documentation.
//
// Construct a Writer with [NewWriter]. Call [Writer.Close] when the
// client disconnects to stop receiving events.
type Writer struct {
	w     io.Writer
	unsub func()
}

// NewWriter creates a Writer that subscribes to src and writes protocol
// lines to w. The caller must call [Writer.Close] when done.
func NewWriter(w io.Writer, src EventSource) *Writer {
	cw := &Writer{w: w}
	cw.unsub = src.Subscribe(cw.handle)
	return cw
}

// Close unsubscribes from the EventSource. After Close returns, no further
// writes will be made to the underlying io.Writer. Close is idempotent.
func (cw *Writer) Close() {
	if cw.unsub != nil {
		cw.unsub()
		cw.unsub = nil
	}
}

// handle dispatches a single event to the appropriate protocol serialiser.
func (cw *Writer) handle(e Event) {
	switch ev := e.(type) {
	case OutputEvent:
		encoded := base64.StdEncoding.EncodeToString(ev.Data)
		fmt.Fprintf(cw.w, "%%output %%%s %s\n", ev.PaneID, encoded)
	case SessionChangedEvent:
		fmt.Fprintf(cw.w, "%%session-changed %s %s\n", ev.SessionID, ev.Name)
	case WindowAddEvent:
		fmt.Fprintf(cw.w, "%%window-add %s\n", ev.WindowID)
	case WindowCloseEvent:
		fmt.Fprintf(cw.w, "%%window-close %s\n", ev.WindowID)
	case WindowPaneChangedEvent:
		fmt.Fprintf(cw.w, "%%window-pane-changed %s %s\n", ev.WindowID, ev.PaneID)
	case LayoutChangeEvent:
		fmt.Fprintf(cw.w, "%%layout-change %s %s\n", ev.WindowID, ev.Layout)
	case ExitEvent:
		fmt.Fprintf(cw.w, "%%exit %s\n", ev.Reason)
	case BeginEvent:
		fmt.Fprintf(cw.w, "%%begin %d %d %d\n", ev.Time, ev.Number, ev.Flags)
	case EndEvent:
		fmt.Fprintf(cw.w, "%%end %d %d %d\n", ev.Time, ev.Number, ev.Flags)
	}
}
