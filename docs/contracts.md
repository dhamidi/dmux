# dmux contracts

Decisions that cross package boundaries. Package docs are detail; this doc is "what you need to know before writing the first line of code." Each section is short and links to the package doc where the full story lives.

## Wire format

The client/server protocol is a bidirectional stream of length-prefixed binary frames over a single connection — Unix socket on Unix, named pipe on Windows. No file-descriptor passing (not portable to Windows).

See `internal/proto/doc.go` for the full byte layout of every message type. The important cross-cutting facts:

- All multi-byte integers are little-endian.
- Max frame size is 1 MiB. Larger frames are a protocol violation.
- Message type IDs with high bit 0x80 are server-to-client; the rest are client-to-server.
- Every frame has a type-agnostic header: `u32 length`, `u16 type`, `u16 flags`, `u32 reserved`, then payload.

## Identify ordering and command buffering

The client must send `Identify` before any other frame. But the common bootstrap path sends `Identify` immediately followed by a `CommandList` containing `new-session` plus `attach-session`. Both frames can arrive in the server's receive buffer before Identify has been processed.

Server rules:

1. `Identify`: process immediately. Drain the pre-Identify `CommandList` buffer.
2. `CommandList` before Identify: buffer, max 10 entries per client. Exceeding 10 is a protocol violation.
3. Anything else before Identify: protocol violation.

10 is a safety margin; the real bootstrap sends exactly one `CommandList`.

## Command lists, not single commands

A single `CommandList` frame carries 1–64 commands in order. Commands within a list execute sequentially; if one returns `Err`, subsequent commands in the same list are marked `Skipped` and not executed.

This exists so the bootstrap `new-session; attach-session` has correct fail-open semantics: if new-session fails (session exists), attach-session doesn't fire trying to attach to something that wasn't created. Without list semantics, the client would need an ack-per-command round trip for the same guarantee.

Commands in separate `CommandList` frames are independent — no ordering guaranteed across frames.

## cmd.Item: what commands see

`cmd.Item` is an interface defined in package `cmd`, not in `server`. The server implements it on a private struct. This breaks what would otherwise be a `cmd` ↔ `server` import cycle.

The interface surface:

- **Identity**: `Context() context.Context`, `Client() Client`
- **Resolved targets**: `Target() Target`, `Source() Target` (after `-t`/`-s` flag interpretation)
- **World access**: `Sessions() SessionLookup`, `Options() OptionLookup`, `Shutdown()`
- **Continuations**: `Prompt(text) <-chan PromptResult`, `Confirm(text) <-chan bool`
- **Messaging**: `Message(text)` (sends to the calling client's status overlay)
- **Logging**: `Log() *slog.Logger` (pre-tagged with command name + item id)

`SessionLookup`, `OptionLookup`, `Client`, `Target`, `SessionRef`, `WindowRef`, `PaneRef` are all interfaces in package `cmd`. Server supplies concrete implementations. Commands can read session state, write to the status line, prompt the user, log, access scoped options — that's it. If a command needs more, extend the Item interface; don't reach past it.

See `internal/cmd/doc.go`.

## Args via stdlib flag, not struct tags

Each command's Args type implements:

```go
type Args interface {
    Bind(*flag.FlagSet)
    Positional() []string
}
```

The framework constructs a `flag.FlagSet`, calls `Bind` (which registers flags via `fs.BoolVar`, `fs.StringVar`, etc.), then calls `fs.Parse(argv)`, then passes the populated Args to Exec. No reflection, no codegen, no tag dialect — just the standard library.

See `internal/cmd/doc.go` for the full pattern and the generic `cmd.New[Args]` registration helper.

## Errors are structured

Every error returned from a command via `cmd.Err` must be matchable with `errors.Is` or `errors.As`. Use the shared vocabulary in package `cmd`:

```go
ErrNotFound       ErrAmbiguous       ErrInvalidTarget
ErrNotImplemented ErrParseFailure

type TargetError struct { Kind TargetKind; Spec string; Reason error }
type ParseError  struct { Source SourceLoc; Reason error }
```

Rationale: the status line's message formatter inspects errors via `errors.As` to render different styles (red for fatal, yellow for missing-target). Free-form strings defeat this and every command author reinvents the wheel.

See `internal/cmd/doc.go`.

## Logging

Both server and client log to files, not stderr. Stderr on the server is detached; stderr on the client gets multiplexed into the real terminal and garbles rendered output.

- Server: `$XDG_STATE_HOME/dmux/server-<socket-label>.log` (or platform equivalent).
- Client: `$XDG_STATE_HOME/dmux/client-<pid>.log`.

Use `log/slog` with structured attrs, not format strings:

```go
// yes
log.Info("resize", "pane_id", p.ID, "cols", c, "rows", r)

// no
log.Info(fmt.Sprintf("resize pane %d to %dx%d", p.ID, c, r))
```

`-v` raises to Debug; `-vv` raises further with internal tracing.

See `internal/log/doc.go`.

## Resize ordering: vt first, then pty

In the pane goroutine, Resize handling is always:

```go
term.Resize(cols, rows)      // vt gets new dimensions first
pty.Resize(cols, rows)       // then SIGWINCH; shell emits new-sized output
resetViewportToLive()
```

This matches tmux's `screen_resize` → `ioctl(TIOCSWINSZ)` order. Reversed, there's a race where the shell starts emitting new-sized output before the vt is ready for it, briefly corrupting the grid.

See `internal/pane/doc.go`.

## Active pane: shared per window

A window has exactly one active pane. That state is shared across every client attached to every session that contains the window. Changing the active pane (`select-pane`, M3 click-to-focus) is seen by every client on the next render.

Matches tmux. Supports pairing; avoids "which pane am I typing into" ambiguity when two users share a session.

See `internal/session/doc.go`.

## Window-size policy

Sessions with multiple clients of different sizes use the tmux `window-size` option to pick one:

- `latest` (default): size to most recently attached/resized client.
- `largest`, `smallest`: as named.
- `manual`: don't track; `resize-window` only.

Smaller clients see letterboxed content; larger clients see the session plus padding. Exact tmux semantics.

See `internal/session/doc.go`.

## Shutdown: force, not graceful

`kill-server`, last-session-closing with `exit-empty`, or SIGTERM all cancel the root context. No drain timeout:

- Pending Output frames dropped; sockets closed.
- Pty fds closed; pane goroutines exit without waiting for child reap.
- SIGHUP sent to child process groups on Unix; `ConptyClosePseudoConsoleTimeout(h, 0)` on Windows.
- Server process exits; orphan children reaped by init/session leader.

Rationale: dmux holds no persistent state. User's shell state lives in child processes, which survive detach. `kill-server` is explicitly asking to lose that state.

See `internal/server/doc.go`.

## Renderer cache: both grid and bytes

Per-pane-per-client Frame caches store the vt.Grid snapshot AND the bytes emitted to realize it. Redundant, but the diff path needs O(1) "which bytes correspond to cell (x,y)" lookups to stay fast on large grids.

Memory cost is ~70KB per cached Frame; 40 Frames (10 panes × 4 clients) is under 3MB. Accepted in exchange for keeping diff rendering linear in changed cells rather than total cells.

See `internal/termout/doc.go`.

## Testing

Tested via scenario files in the production command language plus seven test-only commands (`wait`, `assert`, `expect`, `at`, `test-attach`, `test-detach`, `test-set-recorder-level`). No mocks, no fakes — real servers, real sockets, real ptys.

Observability through the production flight recorder (`internal/record`). Scenarios wait on recorder events rather than on wall-clock time. Same recorder in production and tests; user bug reports can be replayed as scenarios.

See [testing.md](testing.md) for the full contract.

## Maintenance

When a cross-cutting decision is made or revised, update this file in the same PR. Package docs should link here; this file should link back to package docs for detail. Don't inline full decisions twice; pick the right home and cross-reference.
