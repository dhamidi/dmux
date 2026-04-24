# dmux testing

Testing is integration testing. Scenarios drive a real dmux server in-process with real sockets, real ptys, real libghostty-vt, and observe its behaviour through a built-in flight recorder.

No mocks. No fake ptys. No stubbed wasm. No parallel test API.

## The shape

Three pieces:

1. **Flight recorder.** Every state transition in the server emits a structured event to a ring buffer. In production, these go to the log file. In tests, they feed a matcher engine. Same events either way.

2. **Scenario language.** Plain dmux command language (the same thing `.dmux.conf` speaks) plus a handful of scenario-oriented commands that ship in every build. Scenarios live in `.scenario` files and read like tmux transcripts. No Go-level test API is exposed.

3. **Scenario runner.** A Go test harness that parses scenario files into `cmd.List` values using the real `cmd.Parse`, spawns a real server in-process, attaches synthetic clients via real Unix sockets, and executes the commands — including scenario-oriented waits and assertions — through the real `cmd.Registry`.

## Why scenario = command language

The scenario language *is* the command language. Scenarios are built from real dmux commands (`new-session`, `send-keys`, `attach-session`, `bind-key`) plus a handful of scenario-oriented commands (`wait`, `assert`, `expect`, `client at`, `attach-client`, `detach-client`, `recorder set-level`).

Benefits that fall out for free:

- **Parser reuse.** `internal/cmd.Parse` parses scenarios; we don't maintain a second grammar.
- **Target resolution reuse.** `-t work:0.1` in a scenario means exactly what it means in a running session. The same resolver serves both.
- **Flag parsing reuse.** Scenarios use `flag.FlagSet`-backed Args identical to production.
- **Refactor safety.** If the team renames `-s` on `new-session`, every scenario using that flag breaks. That's correct — it's a user-visible change.
- **Low translation cost.** A user reproducing a bug copies their commands from the `:` prompt straight into a scenario file.
- **Quality forcing function.** If a scenario is hard to write, the command language has a usability problem. We dogfood the interface with every test.

The scenario-oriented commands ship in every build, registered in the same `cmd.Registry` as production commands. They are useful beyond tests: hook scripts use `wait` and `assert`, AI agents drive the server through `attach-client` and `client at`, interactive debuggers raise the event stream with `recorder set-level debug`. Scenarios are one caller among several, not a separate world.

## Scenario file format

Line-oriented, one command per line, `#` line comments. Semicolons and brace groups work because the parser is the real parser:

```scenario
# testdata/scenarios/m1-basic-io.scenario
# A client attaches, runs echo, observes the output.

attach-client =A -F Ghostty -x 80 -y 24
new-session -d -s work
wait pane-ready -t work:0.0
client at =A "echo hello\n"
wait output -t work:0.0 -- "hello"
assert screen -t work:0.0 -- "hello"
```

Scenarios can compose via `;` and `{}`:

```scenario
new-session -d -s work ; new-window -t work -n logs ; wait pane-ready -t work:1.0
```

Scenario lines are executed as a single `cmd.List`. If-fail-then-skip semantics apply within a line; across lines, each line is its own list (failure of a command terminates the scenario with failure). `wait` and `assert` fail the scenario when their predicate doesn't hold within the timeout.

## The scenario-oriented commands

### `attach-client <name> [-F profile] [-x cols] [-y rows]`

Creates a synthetic client. Spawns a goroutine in the caller's process that connects to the real server socket, speaks the wire protocol, identifies with the given profile. From the server's perspective indistinguishable from a real `dmux` client.

`name` is an identifier (`=A`, `=B`) subsequent commands use to refer to this client.

### `detach-client <name>`

Cleanly disconnects the named client. Sends `Bye` on the wire; server emits `Exit{Detached}`; the synthetic client goroutine exits.

### `client at <name> <raw-bytes>`

Injects bytes into the named client's input stream. Same framing as a real `dmux` client's stdin. Escape sequences are written as the real bytes they stand for.

```
client at =A "echo hi\n"
client at =A "\x02d"          # Ctrl-B d
client at =A "\x1bOA"          # up arrow (legacy application cursor keys)
```

`client at` is the only way a scenario drives input. There's no other API.

### `wait <event> [-t target] [-- text-predicate] [-T duration]`

Blocks until the named recorder event matches. Fails the scenario if the timeout (default 5s) expires first. `-t` narrows by target; `--` predicates on text-carrying events; `-T` overrides the timeout.

```
wait pane-ready -t work:0.0
wait output -t work:0.0 -- "prompt>"
wait pane-exited -t work:1.2 -T 30
wait client-gone -t =A
```

Events are from a closed vocabulary. See "Recorder events" below.

### `expect <event> [-t target] [-- text-predicate] -T <duration>`

The absence assertion. Blocks for the duration; fails the scenario if the event *does* appear. Useful for "no bell during this operation" and similar negative-path checks.

```
expect pane-exited -t work:0.0 -T 2
```

### `assert <subject> [-t target] [-y row] [-x col] [-- expected]`

Inspects current state, fails if the expectation doesn't hold. No wait — just snapshot-and-check. Subjects:

- `screen` — content of a pane's live grid (or specific row with `-y`)
- `current-session` — target's current session name
- `current-window` — target client's current window name or index
- `active-pane` — the window's active pane id
- `client-count` — number of attached clients
- `session-count` — number of live sessions
- `pane-dead` — boolean
- `alt-screen` — boolean
- `cursor` — row+col (returned as "y,x")

```
assert screen -t work:0.0 -- "hello"
assert screen -t work:0.0 -y 0 -- "hello"
assert active-pane -t work -- "work:0.0"
assert client-count -- 2
```

### `recorder set-level <normal|debug>`

Scenarios that need to wait on fine-grained events opt in. Keeps the default event stream curated so readable scenarios don't have to filter noise.

## Recorder events

The closed vocabulary. Every event is a structured slog record with a stable name and typed fields.

### Lifecycle

| Event | Fields |
|---|---|
| `server.started` | `socket`, `pid` |
| `server.stopping` | `reason` |
| `client.accepted` | `client` |
| `client.identified` | `client`, `profile`, `cols`, `rows` |
| `client.attached` | `client`, `session` |
| `client.detached` | `client`, `reason` |
| `client.gone` | `client`, `reason` |
| `session.created` | `session`, `name` |
| `session.destroyed` | `session`, `reason` |
| `window.created` | `window`, `session`, `index`, `name` |
| `window.destroyed` | `window`, `reason` |
| `pane.spawning` | `pane`, `window`, `shell`, `argv` |
| `pane.ready` | `pane` |
| `pane.exited` | `pane`, `exit-code`, `reason` |

### Commands

| Event | Fields |
|---|---|
| `cmd.parsed` | `item`, `name`, `argv` |
| `cmd.exec` | `item` |
| `cmd.result` | `item`, `status`, `message` |
| `cmd.awaiting` | `item`, `kind` |
| `cmd.resumed` | `item` |

### I/O and state

| Event | Fields |
|---|---|
| `frame.received` | `client`, `type`, `size` |
| `frame.sent` | `client`, `type`, `size` |
| `pty.input` | `pane`, `bytes` |
| `pty.output` | `pane`, `text` (sampled, max 256 bytes) |
| `pane.resize` | `pane`, `cols`, `rows` |
| `pane.snapshot` | `pane`, `dirty` |
| `vt.mode-changed` | `pane`, `mode`, `value` (alt-screen, app-cursor-keys, KKP enabled, etc.) |
| `key-table.switched` | `client`, `table` |
| `render.start` | `client`, `pane` |
| `render.emitted` | `client`, `bytes` |
| `status.rendered` | `client`, `message` |

### What is *not* in the vocabulary

- Every `vt.Feed` call. Too noisy.
- Every socket byte. Too noisy.
- Every main-loop select iteration. Meaningless to scenarios.

If a scenario needs these, `recorder set-level debug` adds `vt.feed`, `socket.read`, `loop.iter`. Most scenarios stay at normal.

## Target identifiers

Scenario target syntax is the production target syntax, with two additions:

- `$N`, `@N`, `%N` — session, window, pane IDs (matches tmux).
- `name`, `name:index`, `name:index.pane` — human-friendly.
- **`=A`, `=B`, ...** — synthetic-client handles (produced by `attach-client`).
- **`!N`** — command item ID, only appears in recorder events (not scenario-writable).

Sigils:

- `$` session
- `@` window
- `%` pane
- `#` client
- `!` command item
- `=` synthetic-client scenario handle

## Example: multi-client scenario

```scenario
# Two clients attach. One detaches explicitly; the other stays.
# The detaching client sees Exit{Detached}; the remaining client is unaffected.

attach-client =A -F Ghostty -x 80 -y 24
new-session -d -s work
wait pane-ready -t work:0.0

attach-client =B -F WindowsTerminal -x 120 -y 30
attach-session -t work
wait client-attached -t =B

# Now two clients are on the same session.
assert client-count -- 2

# =B detaches.
detach-client =B
wait client-gone -t =B
assert client-count -- 1

# =A should still be attached and functional.
client at =A "echo still here\n"
wait output -t work:0.0 -- "still here"
```

## Example: resize race regression

```scenario
# Regression test: after resize, the vt must be sized before the pty.
# The recorder event ordering pins this.

attach-client =A -F Ghostty -x 80 -y 24
new-session -d -s work
wait pane-ready -t work:0.0

# Trigger a resize on the client. Server propagates to pane.
client at =A "\x1b[8;30;100t"     # xterm CSI t resize notification

# Events must be in this order for the same pane.
wait vt.mode-changed -t work:0.0 -- "resize"
wait pane.resize -t work:0.0

# pty.output after the resize should reflect the new cols.
client at =A "stty size\n"
wait output -t work:0.0 -- "30 100"
```

## Example: command-list fail-skip semantics

```scenario
# A CommandList with a failing first command should skip subsequent ones.

attach-client =A -F Ghostty -x 80 -y 24

# new-session -s bogus\x00 has an invalid name (null byte).
# The following attach-session should be skipped, not executed.
new-session -s "bogus\x00" ; attach-session -t bogus

wait cmd.result -- "status=error"
expect client-attached -t =A -T 500ms
```

## Scenario runner structure

```go
// scenarios_test.go
func TestScenarios(t *testing.T) {
    for _, path := range mustGlob("testdata/scenarios/*.scenario") {
        path := path
        t.Run(filepath.Base(path), func(t *testing.T) {
            t.Parallel()
            dmuxtest.Play(t, path)
        })
    }
}
```

`dmuxtest.Play(t, path)`:

1. Creates a temp directory, starts a dmux server rooted there with its socket in that dir.
2. Opens the recorder stream via an in-process channel subscription.
3. Reads the file, pipes it through `cmd.Parse`, runs each command list through the real cmdq against the real server.
4. Test-only commands in the list use the recorder subscription (for `wait`/`expect`) or direct state reads via the `cmd.Host` interface (for `assert`).
5. On any failure: kills the server, dumps the recent recorder tail filtered by target, marks the test failed with the scenario line number.
6. On success: runs the server's force-shutdown path, verifies no goroutine leaks.

## Failure output

When `wait` or `assert` fails, the error message is the scenario line that failed plus the recorder tail:

```
--- FAIL: TestScenarios/m1-basic-io.scenario (0.14s)
scenario m1-basic-io.scenario:5:
    wait output -t work:0.0 -- "hello"

timeout after 5s. Events at %0 (last 10):

  +0.012s  pane.ready        pane=%0
  +0.018s  pty.input         pane=%0 bytes=13
  +0.024s  pty.output        pane=%0 text="echo hello\r\n"
  +0.025s  pty.output        pane=%0 text="bash: echo: command not found\r\n"
  +0.026s  pty.output        pane=%0 text="$ "

Server state at failure:
  clients: [=A attached session=work]
  sessions: [work (1 window, 1 pane)]
  panes: [%0 window=@0 size=80x24 dead=false]
```

The failing line is shown verbatim so the author doesn't have to count lines. The recorder tail is filtered by the wait's target so irrelevant events from other panes don't drown the signal. The server-state summary helps when the event stream was inconclusive.

`DMUX_SCENARIO_VERBOSE=1` dumps the full recorder stream, unfiltered.

## What the scenario runner is not

- Not a mock seam. Scenarios can observe, not inject behavior.
- Not a replacement for Go unit tests of pure functions. `proto` encoding, `sockpath` resolution, `options` lookup — table tests in Go. Scenarios are the integration layer.
- Not a performance test framework. Scenarios run at real-system speed; performance is measured separately.

## What M1 needs

Before writing any production code:

1. The recorder package (`internal/record`).
2. Emission points across M1's packages (server, pane, cmd, cmdq, pty, vt, client). Roughly 30-40 call sites.
3. The scenario-oriented command packages (`internal/cmd/wait`, `.../assert`, `.../expect`, `.../attachclient`, `.../detachclient`, plus the `at` subcommand in `.../client` and the `set-level` subcommand in `.../recorder`), registered in the same `cmd.Registry` as production commands and shipping in every build.
4. The runner (`internal/dmuxtest`): server-in-process spawn, synthetic-client goroutines over real sockets, scenario file parsing via `cmd.Parse`.
5. One minimal scenario that exercises `new-session` → `wait pane-ready` → `client at` → `wait output` → `assert screen`. Passing this scenario is part of M1's exit criteria.

Subsequent milestones each add scenarios for their features. By M5 the scenario directory is the specification of dmux.

## Production value

The recorder ships in production builds. It writes to the same log file dmux already maintains. When a user files a bug, their last N minutes of recorder events ARE the reproduction steps — paste them into a scenario file and replay. Test infrastructure IS production debugging infrastructure.

## Corresponding tmux practice

tmux has no equivalent. Its test suite is shell scripts that drive `tmux` subprocesses with `expect`-style timing. This design replaces those with structured, event-driven scenarios that run deterministically against an in-process server. dmux gets to skip tmux's "flaky test, please rerun" phase.
