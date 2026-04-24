package termin

import "github.com/dhamidi/dmux/internal/keys"

// Event is one semantic input event emitted by Parser.Feed and
// Parser.Tick. The interface is sealed: only the types defined in
// this package satisfy it. Consumers type-switch on the concrete
// type to dispatch.
//
// The M1 parser only emits KeyEvent, PasteEvent, and MouseEvent
// (mouse with zeroed fields; see doc.go). FocusEvent and
// CapsResponse are defined so downstream code can reference the
// types, but the parser does not yet emit them.
type Event interface {
	isEvent()
}

// KeyEvent wraps a keys.Event. Every semantic keypress observed on
// the client's terminal stream comes out as a KeyEvent.
type KeyEvent struct {
	keys.Event
}

func (KeyEvent) isEvent() {}

// MouseButton identifies which SGR mouse button was pressed.
// Values correspond to the xterm SGR button codes after masking
// off the modifier/motion bits.
type MouseButton uint8

const (
	// MouseButtonLeft is SGR code 0.
	MouseButtonLeft MouseButton = 0
	// MouseButtonMiddle is SGR code 1.
	MouseButtonMiddle MouseButton = 1
	// MouseButtonRight is SGR code 2.
	MouseButtonRight MouseButton = 2
	// MouseButtonNone marks motion or release events with no button.
	MouseButtonNone MouseButton = 255
)

// MouseEvent represents one SGR-encoded mouse event. M1 recognizes
// the envelope so the bytes do not leak through as stray keys, but
// the fields are zero-valued placeholders — routing consumers must
// not rely on them yet.
//
// Not yet populated in M1: Button, X, Y, Mods, Press are placeholders.
type MouseEvent struct {
	// Button is the pressed button, or MouseButtonNone for motion/release.
	Button MouseButton
	// X is the 1-based cell column reported by the terminal.
	X int
	// Y is the 1-based cell row reported by the terminal.
	Y int
	// Mods is the modifier mask at the time of the event.
	Mods keys.Mods
	// Press is true for button-down, false for button-up/motion.
	Press bool
}

func (MouseEvent) isEvent() {}

// FocusEvent reports that the client's terminal window gained or
// lost keyboard focus. Not yet emitted in M1; defined so downstream
// code can reference the type.
type FocusEvent struct {
	// In is true for focus-in, false for focus-out.
	In bool
}

func (FocusEvent) isEvent() {}

// PasteEvent carries the bytes delivered between a bracketed-paste
// start (CSI 200 ~) and end (CSI 201 ~) marker. Data is exactly the
// bytes the terminal sent, with no interpretation.
type PasteEvent struct {
	// Data is the pasted payload, in the terminal's native encoding.
	Data []byte
}

func (PasteEvent) isEvent() {}

// Emission pairs one semantic Event with the raw input bytes that
// produced it. Routing code uses Bytes to decide what to forward
// to an attached pane when the event is not bound to a command:
//
//   - Bound key: the command fires and Bytes is dropped.
//   - Unbound key: the bytes are written verbatim to the pane pty.
//
// Bytes is freshly allocated per emission — it does not share
// backing storage with the Feed input, so callers may reuse their
// input buffer immediately after Feed returns. The slice survives
// beyond the next Feed / Tick call, unlike the enclosing []Emission
// result, which is reset on every entry point.
//
// Bytes is nil for Tick-emitted events (currently only KeyEscape):
// the event is time-driven, with no corresponding input byte on
// this call — the original ESC byte was consumed by an earlier
// Feed. Routing treats nil as "nothing to forward".
//
// For a bracketed paste, Bytes covers the whole envelope — the
// opening "\x1B[200~" marker, the payload bytes, and the closing
// "\x1B[201~" marker — so passthrough forwards the entire paste
// transparently. The inner PasteEvent.Data field remains the
// payload-only slice.
type Emission struct {
	// Event is the semantic event the parser recognized.
	Event Event
	// Bytes is the raw input bytes that produced Event, or nil
	// for Tick-emitted events.
	Bytes []byte
}

// CapsKind identifies which deferred capability response the
// parser observed. Used when a DA/DSR/KKP reply arrives on the
// input stream after the initial Detect probe.
type CapsKind uint8

const (
	// CapsKindUnknown is the zero value.
	CapsKindUnknown CapsKind = 0
	// CapsKindDA1 is a Primary Device Attributes reply.
	CapsKindDA1 CapsKind = 1
	// CapsKindDA2 is a Secondary Device Attributes reply.
	CapsKindDA2 CapsKind = 2
	// CapsKindKKP is a Kitty Keyboard Protocol query reply.
	CapsKindKKP CapsKind = 3
)

// CapsResponse is a deferred DA/DSR/KKP reply observed on the input
// stream. Not yet emitted in M1; defined so downstream code can
// reference the type.
type CapsResponse struct {
	// Raw is the full byte sequence as observed, excluding leading ESC.
	Raw []byte
	// Kind classifies the reply.
	Kind CapsKind
}

func (CapsResponse) isEvent() {}
