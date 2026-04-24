// Package assert implements the assert command.
//
// assert inspects current server state and fails if the expectation
// does not hold. Scenarios use it for state-check steps; hook
// scripts and interactive `:` invocations can use it to query the
// same state through the same interface.
//
// # Synopsis
//
//	assert <subject> [-t target] [-y row] [-x col] [-- expected]
//
// Inspects current server state, fails the scenario if the
// expectation does not hold. No wait; snapshot-and-check only.
//
// # Typed args
//
//	type Args struct {
//	    Subject  string  // positional: screen, current-session, client-count, ...
//	    Target   string  // -t
//	    Row      int     // -y (for screen subject)
//	    Col      int     // -x (for screen subject)
//	    Expected string  // anything after `--`
//	}
//
// # Subjects
//
//   - screen            content of a pane's live grid.
//                       With -y, just that row. With -y and -x, just
//                       that cell. Expected is a substring contained
//                       in the retrieved content.
//   - current-session   target client's current session name.
//   - current-window    target client's current window index or name.
//   - active-pane       target window's or session's active pane id
//                       (formatted as "session:index.pane").
//   - client-count      number of attached clients (integer).
//   - session-count     number of live sessions (integer).
//   - pane-dead         boolean for target pane (true/false).
//   - alt-screen        boolean for target pane.
//   - cursor            "row,col" for target pane.
//
// # Behaviour
//
//  1. Resolve Target through cmd.Item.
//  2. Fetch state via cmd.Item's accessors — Sessions(), Options(),
//     the Target's PaneRef for vt reads.
//  3. Compare to Expected. For strings, substring containment. For
//     numbers and booleans, exact match. For "row,col", exact.
//  4. Return cmd.Ok on match; cmd.Err with AssertionError on
//     mismatch. The error carries subject, target, expected, and
//     actual values for the scenario-failure diagnostic.
//
// # Examples
//
//	assert screen -t work:0.0 -- "hello"
//	assert screen -t work:0.0 -y 0 -- "hello"
//	assert active-pane -t work -- "work:0.0"
//	assert client-count -- 2
//	assert alt-screen -t work:0.0 -- "true"
//	assert cursor -t work:0.0 -- "5,10"
//
// # Why this lives in the cmd registry
//
// `assert` reads live state exactly the way a production command
// like `display-message #{pane_current_command}` does — through the
// cmd.Item interface. Building it this way forces cmd.Item to
// expose enough introspection that real commands (and users running
// `:` commands) can see the same state. If `assert screen` can't
// see a pane, neither can `display-message`, and that's a
// cmd.Item/Host bug to fix — not a test-harness workaround.
//
// # Registration
//
//	var Cmd = cmd.New("assert", nil, exec)
//
//	func init() { cmd.Register(Cmd) }
//
//	func exec(item cmd.Item, a *Args) cmd.Result {
//	    actual, err := read(item, a)
//	    if err != nil { return cmd.Err(err) }
//	    if !matches(actual, a.Expected) {
//	        return cmd.Err(&AssertionError{
//	            Subject: a.Subject, Target: a.Target,
//	            Expected: a.Expected, Actual: actual,
//	        })
//	    }
//	    return cmd.Ok
//	}
//
// # Scope boundary
//
// No state mutation. No waiting. If a test wants "eventually X",
// it's a wait, not an assert. Mixing them would make scenarios
// ambiguous about timing.
package assert
