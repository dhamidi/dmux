// Package popup implements `display-popup` — a floating terminal
// window rendered over the current view.
//
// # Boundary
//
// Implements [modes.ClientOverlay]. Depends on the [Pane] interface
// (a locally-defined subset of [pane.Pane]) and a [PaneFactory]
// function for creating the underlying pane. Neither the concrete
// pane struct nor any PTY library is imported directly.
//
// # Pane interface
//
// [Pane] declares only the methods popup actually calls:
// Write, SendKey, SendMouse, Resize, Snapshot, and Close.
// Any value that satisfies this interface — including pane.Pane from
// a real PTY — can be supplied.
//
// # PaneFactory
//
// [PaneFactory] is a function type:
//
//	type PaneFactory func(rows, cols int, command string) (Pane, error)
//
// It is called once in [New] to create the pane sized to the inner
// area (the popup rectangle minus the one-cell border on each side).
// Tests pass a fake factory; production code passes a factory that
// calls pane.New with real PTY and terminal dependencies.
//
// # Construction
//
//	m, err := popup.New(rect, command, autoClose, factory)
//
// rect is the outer bounding box including the border.
// command is the shell command (or empty string for $SHELL).
// autoClose (reserved) will auto-close the popup when the command exits.
// factory creates the underlying [Pane].
//
// # Rendering
//
// The outer rectangle carries a single-line box border (┌─┐ / │ │ / └─┘).
// The pane's [pane.CellGrid] snapshot fills the interior. All rendering
// happens synchronously inside [Mode.Render]; there is no background
// refresh goroutine in this package.
//
// # Event routing
//
// Escape closes the popup ([modes.CloseMode]). All other keys are
// forwarded to the pane via SendKey. Mouse events inside the
// popup rectangle are forwarded via SendMouse; events outside are
// [modes.Passthrough].
//
// [Mode.CaptureFocus] returns true, so the host routes all keyboard
// events here instead of to the focused pane underneath.
//
// # In isolation
//
// Testable by constructing a popup with a [FakePaneFactory], pumping key
// and mouse events, and asserting on the rendered cells including borders,
// without starting any real process or PTY.
//
// # Non-goals
//
// Not a tabbed multi-popup manager — one popup per client at a time.
// Stacking multiple popups is not a planned feature.
package popup
