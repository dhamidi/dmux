// Package testsetrecorder implements the test-set-recorder-level
// test-only command.
//
// Builds only under the `dmuxtest` build tag.
//
// # Synopsis
//
//	test-set-recorder-level <normal|debug>
//
// Changes the recorder's emission level for the duration of the
// scenario. Keeps the default event stream curated so readable
// scenarios don't have to filter noise.
//
// # Typed args
//
//	type Args struct {
//	    Level string  // positional: "normal" or "debug"
//	}
//
// # Levels
//
//   - normal  Default. The ~30 events listed in docs/testing.md.
//             Roughly one event per user action.
//   - debug   Adds per-byte pty.input, per-frame decode events,
//             per-loop-iteration server.loop events, per-cell
//             render.cell events. Much higher volume; only for
//             scenarios that need it.
//
// # Behaviour
//
//  1. Call record.SetLevel with the specified level.
//  2. Register a scenario-lifetime cleanup to restore Normal.
//  3. Return cmd.Ok.
//
// # When to use debug
//
//   - Scenarios testing parser state-machine transitions byte-by-byte.
//   - Scenarios testing exact render output byte-for-byte.
//   - Bug repros where the failure mode is "something between two
//     normal events."
//
// Don't use debug in scenarios that don't need it. The volume
// makes failure diagnostics less readable, and debug events are
// allowed to be dropped if the recorder channel fills — which will
// cause scenarios relying on them to flake.
//
// # Example
//
//	test-set-recorder-level debug
//	at =A "\x1b[1;5A"
//	wait vt.feed -t work:0.0 -- "escape-start"
//	wait vt.feed -t work:0.0 -- "csi-param"
//	wait vt.feed -t work:0.0 -- "csi-final"
//
// # Registration
//
//	var Cmd = cmd.New("test-set-recorder-level", nil, exec)
//
//	func init() { cmd.Register(Cmd) }
//
// # Scope boundary
//
// Scenario-local. Doesn't persist across scenarios; each
// dmuxtest.Play call resets to Normal.
package testsetrecorder
