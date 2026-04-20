# dmux

A terminal multiplexer in Go, built on [libghostty-vt](https://github.com/ghostty-org/ghostty)
via the official [go-libghostty](https://github.com/mitchellh/go-libghostty) bindings.

Think tmux, but with Ghostty's terminal emulation core instead of a hand-rolled
VT parser, and a Go codebase instead of C.

## Status

Skeleton. The directory layout, package boundaries, and design notes are in
place. Implementation is progressing.

`internal/command/builtin` now contains ~28 built-in commands covering session,
window, pane, client, key-binding, option, scripting, and UI categories.
Each command handler receives a `*command.Ctx` and interacts with the server
exclusively through its `Server` (read) and `Mutator` (write) interfaces — no
builtin file imports any other internal package.

`cmd/dmux` is now a functional entry point. Running `dmux` auto-starts the
server on first use and attaches as a client. Run `dmux <command> [args]` to
issue a specific command (e.g. `dmux new-session`). The binary contains both
roles; `dmux start-server` runs in server mode explicitly.

Mouse support is implemented and gated on the `mouse` session option. When
`mouse` is `on`, the server handles SGR mouse events: left-click focuses the
pane under the cursor, drag on a border resizes panes, and scroll-wheel events
are forwarded to the active pane. When `mouse` is `off` (the default), all
mouse escape sequences are forwarded to the active pane unchanged.

Pane borders are drawn using the `pane-border-lines` window option (`single`,
`double`, `heavy`, `simple`, `padded`). Set `pane-border-status` to `top` or
`bottom` to display a label on the horizontal border above or below each pane.
The label text is controlled by `pane-border-format` (default `#{pane_index}`)
and is expanded using the same `#{...}` format-string syntax used elsewhere.

## Usage

```
dmux                        # attach (auto-starts server if needed)
dmux new-session            # create a new session
dmux list-sessions          # list sessions
dmux start-server           # run server in foreground
```

The server socket path is resolved in priority order:

1. `$DMUX_SOCKET` (if set)
2. `$XDG_RUNTIME_DIR/dmux.sock` (if `$XDG_RUNTIME_DIR` is set)
3. `$(os.UserCacheDir)/dmux/dmux.sock` (fallback)

## Design

The architecture is laid out in tiers. Nothing in a lower tier may import
anything from a higher tier. This constraint is what lets each module be
tested and used in isolation.

```
Tier 5  cmd/dmux                              entry point
Tier 4  server            client                processes
Tier 3  command  status  modes/*  control      interaction
Tier 2  render  format  session  job           composition
Tier 1  pane              layout                domain primitives
Tier 0  proto  pty  term  keys  options         foundation
        parse  shell  osinfo
```

Each package has a `doc.go` describing its boundary, public surface, and what
"in isolation" means for it.

## External dependencies

- `github.com/mitchellh/go-libghostty` — Go bindings to libghostty-vt.
  Provides the terminal emulator that backs every pane. Links libghostty-vt
  statically via cgo; the resulting binary has no runtime deps beyond libc.
- `golang.org/x/sys` — Platform syscalls: `TIOCGWINSZ`/termios on Unix,
  ConPTY/`SetConsoleMode`/`GetConsoleScreenBufferInfo` on Windows.

No other third-party dependencies.

## Platform support

Primary targets: Linux and macOS.
Secondary target: Windows 10 1803+ running in Windows Terminal.
Legacy `conhost.exe` is not supported — run the Windows client inside Windows
Terminal (or WSL on older Windows).

## Non-goals

- Drop-in tmux config compatibility. The command language and format strings
  will look familiar but are not promised bug-for-bug compatible.
- Plugin system.
- Anything SIXEL / Kitty-graphics beyond what libghostty-vt gives us for free.

## Directory layout

See individual `doc.go` files under `internal/` for per-package details.
Higher-level design notes that span packages live under `docs/`.
