// Package previouswindow implements the previous-window command.
//
// # Synopsis
//
//	previous-window
//
// # Typed args
//
//	type Args struct {
//	    AlertOnly bool   `dmux:"a"        help:"rewind only to windows with an alert"`
//	    Target    string `dmux:"t=target" help:"target session"`
//	}
//
// Milestone one implements no flags: the command rewinds the
// caller's current session's window cursor by one, wrapping from
// the first window back to the last.
//
// Deferred (not on the struct yet): AlertOnly, Target.
//
// # Behavior
//
//  1. Read the caller's current session off the Item. A nil session
//     means the connection is not attached; return
//     cmd.Err(ErrNotFound).
//  2. Ask the server to move the cursor by -1 via
//     Item.AdvanceWindow. The server resolves the ref, calls
//     PreviousWindow once, and returns the new current window's
//     ref. A session with no windows surfaces as ErrNotFound.
//  3. Return cmd.Ok. The render tick after this command drains
//     will paint whichever window is now current.
//
// # Wrap-around semantics
//
// previous-window wraps: rewinding past the first window returns
// the last. Matches tmux's default behaviour. A single-window
// session is a no-op (AdvanceWindow returns the sole window).
//
// # Error sentinels
//
// The M1 no-arg case uses cmd.ErrNotFound when CurrentSession is
// nil or when the session has no windows. Future -t support will
// add cmd.ErrInvalidTarget on malformed target specs.
//
// # Registration
//
//	const Name = "previous-window"
//
//	type command struct{}
//
//	func (command) Name() string { return Name }
//	func (command) Exec(item cmd.Item, _ []string) cmd.Result { ... }
//
//	func init() { cmd.Register(command{}) }
//
// # Corresponding tmux code
//
// cmd-select-window.c (the -p variant).
package previouswindow
