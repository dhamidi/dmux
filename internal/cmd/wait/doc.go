// Package wait implements the wait command.
//
// wait is scenario-oriented: it blocks until a named recorder event
// appears, failing on timeout. Scenarios use it for synchronization;
// hook scripts and interactive `:` invocations can use it too.
//
// # Synopsis
//
//	wait <event> [-t target] [-- text-predicate] [-T duration]
//
// Blocks until the named recorder event matches the optional target
// and text predicate. Fails the scenario if the timeout (default 5s)
// expires first.
//
// # Typed args
//
//	type Args struct {
//	    Event          string        // positional; pane.ready, pty.output, etc.
//	    Target         string        // -t
//	    Timeout        time.Duration // -T
//	    TextPredicate  string        // anything after `--`
//	}
//
// Event names are from the closed vocabulary in docs/testing.md:
// `pane.ready`, `pane.exited`, `pty.output`, `client.attached`,
// `client.gone`, `session.created`, `cmd.result`, etc.
//
// # Behaviour
//
//  1. Resolve Target through cmd.Item's resolver (no special-casing
//     for test commands).
//  2. Subscribe to record events; filter by Event name, target match,
//     and if TextPredicate is set, substring containment against the
//     event's text field.
//  3. Return cmd.Await on the filtered event channel with a context
//     deadline of Timeout.
//  4. If the channel fires: cmd.Ok.
//  5. If the context deadlines: cmd.Err with a structured TimeoutError
//     carrying the subscription's recent event tail for the diagnostic.
//
// # Uses cmd.Await
//
// `wait` is a continuation-based command. It doesn't block the
// cmdq — the cmdq parks the item on the subscribe channel and
// resumes when the fan-in goroutine fires. This means multiple
// `wait` commands in different scenarios run concurrently without
// tying up the cmdq.
//
// # Examples
//
//	wait pane.ready -t work:0.0
//	wait pty.output -t work:0.0 -- "hello"
//	wait pane.exited -t work:1.2 -T 30s
//	wait client.gone -t =A
//	wait cmd.result -- "status=error"
//
// # Registration
//
//	var Cmd = cmd.New("wait", nil, exec)
//
//	func init() { cmd.Register(Cmd) }
//
//	func exec(item cmd.Item, a *Args) cmd.Result {
//	    ch, cancel := subscribe(item, a)
//	    defer cancel()
//	    return cmd.Await(ch, func(e record.Event) cmd.Result {
//	        return cmd.Ok
//	    })
//	}
//
// # Scope boundary
//
// No polling; the subscription is event-driven. No injection; wait
// observes, does not manipulate. No complex compound matchers
// beyond "event name + target + optional text"; scenarios needing
// ordering guarantees chain multiple waits.
package wait
