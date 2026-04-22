// Package cmdq implements the command queue that drives command
// execution on the server.
//
// # Why a queue
//
// Commands in tmux are not plain function calls. They can:
//
//   - block (confirm-before, command-prompt) and resume later when
//     the user answers;
//   - chain ("split-window ; send-keys foo Enter");
//   - be scheduled from callbacks (key dispatch, hook firing,
//     post-pane-close cleanup).
//
// All command execution flows through a queue drained by the server's
// main goroutine. The queue itself owns no state beyond the pending
// items; the server's main loop is the single state owner that
// commands mutate.
//
// # Types
//
//	Item    a single queued command, with its parsed args, target
//	        state (session / window / pane), and source state.
//	List    a per-client (or global) sequence of Items.
//	Result  what an Exec returns: Done, Err, or Await.
//
// # Interface
//
//	(*List) Append(Item)
//	(*List) Drain() int                    // runs ready items; returns # run this tick
//	NewItem(c cmd.Command, target Target) Item
//
// Drain runs items until the queue is empty or the head item returns
// an Await. An Await suspends the queue at that item.
//
// # Continuations replace explicit Wait
//
// In tmux, a command that needed to wait set a CMDQ_WAITING flag and
// the queue infrastructure managed park/resume bookkeeping. C requires
// this because functions cannot suspend.
//
// Go has closures and generics. dmux uses continuation-passing:
//
//	func (c *Command) Exec(item *cmdq.Item) cmd.Result {
//	    return cmd.Await(item.Prompt("Sure? "), func(yes bool) cmd.Result {
//	        if !yes { return cmd.Ok }
//	        return item.Run(theInnerCommand)
//	    })
//	}
//
// `cmd.Await[T](ch <-chan T, then func(T) cmd.Result) cmd.Result` is
// a generic helper. The Result it returns wraps an opaque awaitState
// holding the typed channel and the typed continuation.
//
// When Drain sees an awaiting Result, it:
//
//  1. Hands the awaitState to a small fan-in goroutine that performs
//     a single `select { case v := <-ch: ... case <-ctx.Done(): ... }`.
//  2. Returns control to the main loop. The List remains parked at
//     the head item; subsequent items in this List are blocked.
//     Other Lists continue.
//  3. When the channel fires, the fan-in goroutine pushes a typed
//     ContinuationReady event to the server's events channel.
//  4. The main loop receives the event, dispatches to the parked
//     List, runs the continuation. The continuation either returns
//     Done / Err (the item is finished) or another Await (park
//     again).
//
// If the owning context cancels first, the fan-in goroutine drops
// the awaitState without running the continuation. The user-visible
// effect is "the command was abandoned because the client detached."
//
// # Cmdq does not own waiting state
//
// There is no "waiting" flag on Item. There is no state-machine
// transition table. A parked continuation is simply a closure
// referenced by an awaitState referenced by an in-flight goroutine.
// When the goroutine completes (normally or via cancellation), the
// reference graph collapses and the closure is GC'd.
//
// This means cmdq's surface is small: append items, drain items,
// route continuations. The ceremony tmux's cmd-queue.c carries to
// manage Wait flags, group state, and item ordering across waits is
// not reproduced — it has no Go counterpart, because Go closures
// capture what tmux stored explicitly.
//
// # Scope boundary
//
// Owns scheduling and execution orchestration only. Commands
// themselves live in internal/cmd. Their side effects live in
// internal/session, internal/pane, etc. The fan-in goroutine and
// events-channel routing live in internal/server.
//
// # Corresponding tmux code
//
// tmux's cmd-queue.c, but considerably smaller. Item lifecycle,
// reference counting, and the CMDQ_WAITING / CMDQ_FIRED flag
// machinery are absent because closures and GC handle their work.
package cmdq
