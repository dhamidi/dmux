// Package dmuxtest is the scenario runner.
//
// dmuxtest ships in every build. `_test.go` files across the tree
// drive it through Play(t, path) and PlayInline(t, name, script);
// the package is unexported-by-convention (no production caller
// should import it), but it is not gated by a build tag. The name
// dmuxtest is a semantic commitment: this package exists to run
// scenarios against a real server, nothing else.
//
// # What it does
//
// Given a `.scenario` file:
//
//  1. Starts a real dmux server in-process in a temp directory with
//     a socket in that directory.
//  2. Subscribes to the recorder event stream.
//  3. Parses the scenario via the real cmd.Parse into a cmd.List.
//  4. Runs each command through the real server's cmdq against the
//     real state. Scenario-oriented commands (wait, assert, expect,
//     client at, attach-client, detach-client, recorder set-level)
//     are registered in the same cmd.Registry as production
//     commands and execute through the same path.
//  5. On scenario completion, shuts the server down via the real
//     force-shutdown path, verifies no goroutine leaks.
//  6. On failure, dumps the failing line, the recorder tail filtered
//     by target, and a server-state summary.
//
// # Server in-process, not as subprocess
//
// The scenario runner calls `server.Run(cfg)` in a goroutine. No
// fork, no exec. The server's socket is a real AF_UNIX socket in a
// tempdir on both Unix and Windows. Synthetic clients connect to it
// with real `net.Dial`.
//
// Running in-process is what makes the recorder subscription work:
// the test goroutine and the server goroutine share the same
// process address space, so `record.Subscribe` returns a channel
// the test can read directly. It also simplifies goroutine-leak
// checks — the test can snapshot `runtime.NumGoroutine` before and
// after.
//
// # Synthetic clients
//
// `attach-client =A ...` spawns a goroutine inside the test process
// that:
//
//   - Dials the server's socket with net.Dial.
//   - Sends an Identify frame with the given profile and size.
//   - Sends CommandList frames as directed by subsequent `at` and
//     command invocations aimed at =A.
//   - Reads Output frames and keeps a client-side screen buffer by
//     feeding them through a real termin.Parser + vt.Terminal (the
//     same stack the production client uses).
//   - Exits on Bye / Exit / detach-client.
//
// The synthetic client is not a fake. It is a real dmux client
// minus the real terminal — its stdout goes to a byte buffer
// instead of a tty. Everything from the server's perspective is
// identical to a real dmux binary connecting from the command line.
//
// This matters because "attach two clients and detach one" must
// exercise the real per-client reader goroutine, the real writer
// goroutine, the real context-cancellation cascade. If the
// synthetic client were a stub, those code paths would go untested.
//
// # The `assert` command reads live state
//
// `assert screen -t %0 -- "hello"` needs to read the live grid of
// pane %0. This goes through cmd.Item's PaneRef, the same
// interface every production command uses — assert is just another
// command. It doesn't reach past the Item boundary.
//
// This is what forces the cmd.Host interface to be sufficient for
// introspection. If `assert screen` can't see the pane, no
// production command can either, and that's a cmd.Host bug.
//
// # Runner API
//
//	Play(t *testing.T, path string)                        // drive one scenario file
//	PlayInline(t *testing.T, name, script string)          // scenario as string literal
//
// `Play` is the common entry. `PlayInline` is for tests that want
// to construct small scenarios programmatically or for unit tests
// of individual scenario-oriented commands.
//
// # Failure diagnostics
//
// On scenario failure, `Play` calls t.Fatalf with:
//
//	scenario <path>:<line>:
//	    <command text>
//
//	<reason: timeout | assertion mismatch | command error>
//
//	Events at <target> (last 10):
//	    +0.012s  pane.ready  pane=%0
//	    +0.018s  pty.input   pane=%0 bytes=13
//	    ...
//
//	Server state at failure:
//	    clients: [...]
//	    sessions: [...]
//	    panes: [...]
//
// The event tail is filtered by the failing command's target (if
// any). `DMUX_SCENARIO_VERBOSE=1` disables filtering and dumps the
// full stream.
//
// # Goroutine-leak check
//
// After the scenario's force-shutdown completes, the runner waits
// up to 100ms for goroutine count to return to the pre-Play
// baseline. If it doesn't, the scenario fails with a dump of the
// remaining goroutine stacks.
//
// This catches two common M1/M2 bugs at test time:
//
//   - Forgotten ctx.Done handling in a goroutine.
//   - Continuation goroutine (cmd.Await fan-in) orphaned by a
//     context cancel that the continuation's sender didn't observe.
//
// # Parallel scenarios
//
// Each Play call uses its own tempdir, its own socket, its own
// in-process server. Scenarios are safely parallel at the go-test
// level via t.Parallel(). Running 50 scenarios concurrently on an
// 8-core machine is routine.
//
// # Scope boundary
//
//   - No mocks, no fakes, no stubs. Real components throughout.
//   - No assertion framework beyond what the scenario language
//     expresses via the `assert` command. If a test wants a Go-level
//     assert, it's doing something the scenario language should
//     probably express.
//   - No fixture libraries. Scenarios are self-contained; common
//     setup goes in a leading block of the scenario file.
//
// # Interface summary
//
//	Play(t *testing.T, path string)
//	PlayInline(t *testing.T, name, script string)
//
//	// Mostly-internal:
//	SpawnServer(t *testing.T) *Harness
//	(*Harness) AttachSynthetic(name string, cfg ClientConfig) *SyntheticClient
//	(*Harness) Run(cmdLine string) error
//	(*Harness) Record() <-chan record.Event
//	(*Harness) Close()
package dmuxtest
