# dmux milestones

| # | Title | Status |
|---|---|---|
| [M1](m1.md) | Minimal vertical slice: server, client, one session, one window, one pane running a shell, status line | concrete |
| [M2-1](m2-1.md) | Real input parser and capability detection (no user-visible behaviour change) | concrete |
| [M2-2](m2-2.md) | Prefix key, default bindings, detach/attach, multiple windows | concrete |
| [M3](m3.md) | Splits and layouts, mouse, diff rendering | likely shape |
| [M4](m4.md) | Copy mode, scrollback navigation, paste buffers, OSC 52 clipboard | speculative |
| [M5](m5.md) | Format strings, hooks, full options + `.dmux.conf`, status-line styling | speculative |

"Concrete" milestones have per-package work itemised and exit criteria. "Likely shape" and "speculative" milestones state goals and likely scope; details firm up as the preceding milestone ships.

## Cross-cutting references

[**go-patterns.md**](go-patterns.md) — the Go-specific patterns dmux uses across packages: per-pane goroutines under a single-state-owner main loop, continuations for command pauses, `context.Context` for cancellation, typed command arguments, maps-over-trees. Read once before changing `server`, `cmdq`, `cmd`, `pane`, `pty`, or `client`.

[**contracts.md**](contracts.md) — cross-cutting decisions that affect multiple packages: wire format, identify-race, command-list semantics, logging policy, error vocabulary, resize ordering, active-pane sharing, window-size policy, shutdown semantics, renderer cache shape. Read this before the first PR; the decisions are load-bearing.

[**testing.md**](testing.md) — how dmux is tested. A flight recorder plus a scenario language that reuses the production command parser. Scenarios run real servers against real sockets and real ptys; no mocks, no fakes. The test-only commands are registered in the same `cmd.Registry` as production commands under the `dmuxtest` build tag.

## Package tree

```
cmd/dmux/                         entry point, flag parsing, dispatch

internal/proto/                   wire message types and codecs
internal/xio/                     length-prefixed frame reader/writer
internal/socket/                  Unix socket + Windows named-pipe transport
internal/sockpath/                resolve socket path from -L/-S/$DMUX
internal/platform/                daemonize, signals, user context

internal/client/                  client process: dumb byte pump
internal/tty/                     client-side real terminal, raw mode, resize

internal/termin/                  input byte-stream parser (the one we own)
internal/termout/                 diff renderer, per-profile output encoder
internal/termcaps/                5-profile capability matrix + detection

internal/server/                  accept loop, dispatch, main-loop state owner
internal/session/                 Session/Window/Winlink registry
internal/pane/                    one VT + PTY pair, owned by its own goroutine
internal/pty/                     openpty+fork on Unix, ConPTY on Windows
internal/vt/                      libghostty-vt over wazero

internal/keys/                    KeyCode, bindings, tables
internal/cmdq/                    command queue with continuations
internal/cmd/                     Command interface, parser, registry, args framework
internal/cmd/newsession/          concrete command
internal/cmd/attachsession/       concrete command
internal/cmd/killserver/          concrete command

internal/log/                     slog-based logging (both server and client)
internal/options/                 hierarchical scoped options (server/session/window/pane)
internal/status/                  status line renderer

internal/record/                  structured event recorder (flight recorder)
internal/dmuxtest/                scenario runner (build tag: dmuxtest)
internal/cmd/wait/                test-only command (build tag: dmuxtest)
internal/cmd/assert/              test-only command
internal/cmd/expect/              test-only command
internal/cmd/at/                  test-only command
internal/cmd/testattach/          test-only command
internal/cmd/testdetach/          test-only command
internal/cmd/testsetrecorder/     test-only command
```

Thirty-three packages total. Twenty-six production, seven test-only. The test-only packages build only under the `dmuxtest` build tag and never ship in release binaries.

## Principles across milestones

**Narrow interfaces, deep implementations.** Every package boundary is set in M1 and held stable. Later milestones expand what packages do, not what they are.

**Self-hosting command language.** Default key bindings, config files, and programmatic server state are all driven through the same `cmd.Parse` → `cmdq.List` → `Exec` path. One code path is exercised by every entry point.

**Closed target list.** The three supported terminals (Ghostty, xterm.js, Windows Terminal) plus an `Unknown` fallback are a closed set. Supporting another terminal well is a deliberate addition.

**One multiplexer, three platforms.** Linux, macOS, Windows are all first-class from M1. Platform-specific code is confined to three packages (`platform`, `pty`, `socket`). Callers never branch on `GOOS`.

**Per-pane goroutines.** Heavy output in one pane never blocks any other pane or the server's main loop. Each pane has its own goroutine that exclusively owns its `vt.Terminal`. The single-state-owner discipline applies to *server* state (sessions, clients, key tables, command queue) — not to per-pane terminal state.
