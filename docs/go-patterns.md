# Go patterns dmux relies on

dmux is modeled on tmux's architecture but written in Go. Some tmux design choices reflect C limitations rather than design intent. Where Go gives us a better tool, we use it. This document articulates the five patterns that show up across packages so contributors can recognise them.

These patterns are not optional flourishes. They appear in `server`, `cmdq`, `cmd`, `pane`, `pty`, and `client`. Read this once before changing those packages.

## 1. Per-pane goroutines under a single-state-owner main loop

Two layers of state ownership, each with one writer:

**Server state** is owned by the server's main goroutine. The
session/window/winlink registry, per-client state, key tables, and
the command queue all live there. Around it sit *edge goroutines*,
one per I/O source:

- One reader and one writer per attached client (socket I/O).
- One signal handler.
- One ticker.
- One accept goroutine.

Edge goroutines own no shared state. They translate I/O into typed
events and push them onto a single `events chan Event` that the main
goroutine selects on:

```go
for {
    select {
    case ev := <-events:
        // run handler for ev; mutates server state freely
    case sig := <-signals:
        // ...
    case <-ticker.C:
        // ...
    case <-shutdown:
        return
    }
}
```

**Per-pane terminal state** is owned by per-pane goroutines. Each
pane has its own goroutine that exclusively holds its `vt.Terminal`,
its `vt.KeyEncoder`, and its current viewport. A separate helper
goroutine reads from that pane's pty fd and forwards bytes to the
pane goroutine.

This split exists for one reason: heavy pane output (`yes`, `cat`,
build logs) must not block the server's main loop or any other pane.
With one main goroutine doing all the work, a 100MB/s pane would
make the entire multiplexer unresponsive. Per-pane goroutines mean
`vt.Feed` for pane A runs in parallel with `vt.Feed` for pane B, and
neither blocks key handling, status updates, or `kill-server`.

Two consequences of the split:

- The main goroutine never touches a pane's `vt.Terminal` directly.
  It sends control messages on the pane's `controlCh`; snapshots come
  back via push-on-dirty over a shared `snapshotCh`, multiplexed via
  a fan-in goroutine into the main events channel.
- Pane goroutines never touch server state. They have a local
  snapshot of "which client profiles are attached" pushed to them
  when clients come and go.

Edge goroutines and pane goroutines can share the rule: don't reach
into someone else's state graph. Translate to a message and let the
owner handle it.

Two consequences of the discipline:

- No locks anywhere. There is exactly one writer per state domain.
- Any code reaching across goroutines into a state graph it doesn't
  own is a bug.

## 2. Continuations for waiting commands

Some commands need to pause. `confirm-before` waits for y/n. `command-prompt` waits for a typed line. `if-shell` waits for a shell exit.

In tmux, this is implemented as queue-state flags (`CMDQ_WAITING`) and an explicit park/resume protocol. C requires it because functions can't suspend.

Go has closures and generics, so dmux uses continuations:

```go
func (c *Command) Exec(item *cmdq.Item) cmd.Result {
    return cmd.Await(item.Prompt("Really? (y/n) "), func(answer bool) cmd.Result {
        if !answer {
            return cmd.Ok
        }
        return item.Run(theInnerCommand)
    })
}
```

`cmd.Await[T](ch <-chan T, then func(T) cmd.Result) cmd.Result` is the whole infrastructure. The command returns immediately; cmdq detects the await, registers a one-shot goroutine that funnels the channel value back into the main loop's `events` channel, and runs the continuation when ready.

The cmdq doesn't store explicit "waiting" state. A parked continuation is just a closure referenced by an `awaitState` value referenced by an in-flight goroutine. When the channel fires, the closure runs; when the owning client's context cancels, the closure is dropped without running.

## 3. context.Context for cancellation

Every long-running goroutine takes a `context.Context`. The server holds a root context; each client gets a child context derived from it; each pane gets a child of the client (or session, depending on ownership); each pty reader gets a child of the pane.

Cancellation cascades. Detaching a client cancels its context, which cancels its writer goroutine, which closes the socket, which causes the reader goroutine to error out, which propagates to its child contexts. No flags, no `CLIENT_DEAD` state, no carefully ordered destruction.

Where blocking I/O can't observe `ctx.Done()` directly (a `Read` blocked on a pty fd), the cancellation path is: close the fd. The Read returns an error. The goroutine exits.

## 4. Typed command arguments

tmux's `args.c` parses arg specs like `"Adc:n:s:"` into an untyped struct queried by flag character: `args_get(args, 's')`. Compile-time checks don't catch typos.

dmux uses a typed struct per command, with reflection-based parsing driven by struct tags:

```go
type Args struct {
    Detached  bool     `dmux:"d"        help:"don't attach"`
    Attach    bool     `dmux:"A"        help:"attach if exists"`
    Name      string   `dmux:"s=name"   help:"session name"`
    StartDir  string   `dmux:"c=path"   help:"start directory"`
    Command   []string `dmux:"$"        help:"shell command and arguments"`
}

func init() {
    cmd.Register(cmd.New("new-session", exec))
}

func exec(item *cmd.Item, args *Args) cmd.Result {
    if args.Detached { ... }
    if args.Name != "" { ... }
    return cmd.Ok
}
```

Tag syntax: `dmux:"X"` is a boolean flag, `dmux:"X=name"` takes a value, `dmux:"$"` collects positional arguments. The `args` sub-package handles parsing and validation; commands declare and consume.

The command-language *user syntax* is unchanged. `new-session -d -s work` parses identically to tmux. The change is purely internal: the command sees `args.Detached == true && args.Name == "work"` instead of `args_has(a, 'd') && args_get(a, 's') == "work"`.

## 5. Maps and slices, not RB trees

tmux uses OpenBSD's `RB_HEAD`/`RB_ENTRY` macros for every ordered collection because C lacks generics. dmux uses plain `map[ID]*T` and sorts when ordered iteration is needed:

```go
// session.go
type Registry struct {
    sessions map[SessionID]*Session
    windows  map[WindowID]*Window
    panes    map[PaneID]*Pane
}

func (r *Registry) ListSessions() []*Session {
    out := slices.Collect(maps.Values(r.sessions))
    slices.SortFunc(out, func(a, b *Session) int {
        return cmp.Compare(a.Name, b.Name)
    })
    return out
}
```

Iteration uses range-over-func where the natural shape is "give me one at a time" rather than "here's a slice":

```go
func (w *Window) Panes(yield func(*Pane) bool) {
    for _, p := range w.panes {
        if !yield(p) { return }
    }
}

// caller:
for p := range win.Panes {
    // ...
}
```

## What stays the same as tmux

These patterns don't change the architecture documented in the package `doc.go` files. They change the implementation language. The seams are unchanged:

- Single-state-owner stays. The main goroutine *is* the single state owner; we just spelled it with goroutines instead of `select(2)`.
- The command queue stays. Items still go through a `cmd.Command` registered at init. Continuations replace explicit Wait flags but the queue itself still exists.
- Session/Window/Winlink/Pane object graph stays. Maps replace RB trees but the relationships are identical.
- `cmd.Register` at init time stays. Each command sub-package is a separately-readable unit.

If a contributor asks "should this be a goroutine?" the answer is almost always: yes, if it owns I/O; no, if it touches the state graph.
