// Package prompt implements `command-prompt` and `confirm-before`
// — single-line text input dialogs rendered at the bottom of the
// client viewport.
//
// # Boundary
//
// Implements modes.ClientOverlay. Two constructors:
//
//	NewCommand(prompt string, template string, onSubmit func(string) Outcome)
//	NewConfirm(prompt string, onYes Outcome)
//
// Both draw as a one-line strip overlaying the status line area (or
// replacing it). NewCommand supports full line editing (cursor
// movement, word-wise delete, history), tab completion via a
// caller-supplied Completer interface, and %-escape substitution in
// the template on submit.
//
// NewConfirm accepts y / Y / Enter as yes, anything else as no.
//
// # History
//
// Per-prompt-type history is stored in package session (paste buffers
// and prompt history live alongside each other), not here. This
// package takes a history slice at construction and returns the
// updated slice on close.
//
// # In isolation
//
// Testable by constructing a prompt, driving Key calls with a
// simulated typed string, asserting on the completed submit value
// or the produced Outcome.
//
// # Non-goals
//
// No multi-line editing. Anything needing a real text editor should
// shell out to $EDITOR via a normal pane or popup.
package prompt
