# Architecture

gomux is a client-server terminal multiplexer. The server is a long-lived
process that owns all pane terminals, sessions, and windows. Clients are
thin processes that connect over a UNIX-domain socket, forward stdin
bytes to the server, and display the bytes the server sends back.

## Tiers

Packages are organized in strict dependency tiers. Lower tiers may not
import from higher tiers. Sideways imports within a tier are also
avoided where possible.

```
Tier 5  cmd/gomux                              entry point
Tier 4  server            client                processes
Tier 3  command  status  modes/*  control      interaction
Tier 2  render  format  session  job           composition
Tier 1  pane              layout                domain primitives
Tier 0  proto  pty  term  keys  options         foundation
        parse  shell  osinfo
```

The tier structure is what makes every package testable in isolation.
You can boot a `session.Server` with no I/O, expand format strings
against a `map[string]string`, run a `layout.Tree` through resize
arithmetic without any panes existing, and so on.

## External dependency

`github.com/mitchellh/go-libghostty` is the only third-party dependency
that matters. It provides the terminal emulator that backs each pane:
VT parser, grid, scrollback, Unicode, Kitty protocols, render state.
Everything tmux does in `input.c`, `grid.c`, `screen.c`, `utf8.c`,
`colour.c`, and friends is handled by this library.

It's cgo and links `libghostty-vt` statically. Build-time dependency on
a Zig-built library; runtime dependency on libc only.

## Process model

```
     tty                     UNIX socket
      │                           │
      ▼                           ▼
  ┌───────┐  proto.Message   ┌────────┐    ┌─────────┐
  │client │◄────────────────►│ server │◄──►│ panes:  │
  └───────┘                  └────────┘    │ PTY +   │
                                │          │ ghostty │
                                ▼          │ Term    │
                        render.Frame       └─────────┘
                             ▼
                       back on the wire
                       as output bytes
```

Clients do no parsing, no rendering, no state. The server parses
everything (through go-libghostty for pane contents, through
`internal/parse` for the command language), maintains all state, and
composes each client's view by calling `render.Compose` and streaming
the resulting escape sequences back.

## Cross-platform

Linux, macOS, and Windows 10 1803+ in Windows Terminal. Three packages
have platform-specific implementations behind a shared Go interface:

- `internal/pty` — forkpty vs ConPTY
- `internal/term` — terminfo vs hardcoded xterm emit
- `internal/osinfo` — procfs / sysctl / NtQueryInformationProcess

Everything else is platform-neutral Go.

## Protocol

See `internal/proto/doc.go` for the full message list. Highlights:

- `IDENTIFY_*` handshake sends `$TERM`, terminfo, cwd, env, feature
  flags so the server can emit the right escapes and spawn new panes
  with the right environment.
- `READ_*` / `WRITE_*` is a small RPC so the server can ask the client
  to read from its stdin or write to its stdout — powers
  `load-buffer -`, `display-message -p`, etc. without fd-passing
  (which doesn't exist on Windows).
- `RESIZE` is polled on the client side so the protocol is uniform
  across OSes where `SIGWINCH` may or may not exist.
