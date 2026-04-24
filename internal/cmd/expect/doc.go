// Package expect implements the expect command.
//
// expect is the absence assertion: it blocks for a bounded duration
// and fails the scenario if the named event does appear. Scenarios
// use it for negative-path checks; any caller that wants "this must
// not happen during the next N seconds" can use it the same way.
//
// # Synopsis
//
//	expect <event> [-t target] [-- text-predicate] -T <duration>
//
// The absence assertion. Blocks for the duration; fails the
// scenario if the event *does* appear. Use for negative-path checks
// like "no bell during this operation" or "pane does not exit".
//
// # Typed args
//
//	type Args struct {
//	    Event          string
//	    Target         string
//	    TextPredicate  string
//	    Timeout        time.Duration  // -T, required
//	}
//
// Unlike `wait`, Timeout is required — it defines the observation
// window. Default would be ambiguous (is it "wait 5s for absence"
// or "fail immediately"?), so we force explicit.
//
// # Behaviour
//
// Inverse of `wait`:
//
//  1. Resolve Target.
//  2. Subscribe to recorder events filtered by Event + Target + text.
//  3. Return cmd.Await with deadline Timeout.
//  4. If the channel fires before deadline: cmd.Err with
//     UnexpectedEventError carrying the offending event.
//  5. If the deadline elapses without firing: cmd.Ok.
//
// # Examples
//
//	# Pane shouldn't exit for 2 seconds.
//	expect pane.exited -t work:0.0 -T 2s
//
//	# No bell during bulk output.
//	at =A "cat bigfile\n"
//	expect vt.bell -t work:0.0 -T 5s
//
//	# Resize event shouldn't fire on key input alone.
//	at =A "j"
//	expect pane.resize -t work:0.0 -T 500ms
//
// # Why not fold this into wait with a flag?
//
// Considered `wait -n pane.exited` for "wait for non-appearance."
// Rejected because the two commands have different exit semantics
// (wait succeeds on event, fails on timeout; expect does the
// opposite) and sharing a name makes scenarios harder to read.
// Separate verbs, separate packages, separate failure messages.
//
// # Registration
//
//	var Cmd = cmd.New("expect", nil, exec)
//
//	func init() { cmd.Register(Cmd) }
//
// # Scope boundary
//
// Bounded time only. No "forever" mode. Scenarios with unbounded
// expectations are a design smell — if something must never happen,
// bound it to "never during this scenario's lifetime."
package expect
