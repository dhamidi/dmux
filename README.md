# dmux

A terminal multiplexer in Go, built on [libghostty-vt](https://github.com/ghostty-org/ghostty)
via the official [go-libghostty](https://github.com/mitchellh/go-libghostty) bindings.

Think tmux, but with Ghostty's terminal emulation core instead of a hand-rolled
VT parser, and a Go codebase instead of C.

## Status

Skeleton. The directory layout, package boundaries, and design notes are in
place. Implementation is progressing.

`internal/command/builtin` now contains ~40 built-in commands covering session,
window, pane, client, key-binding, option, scripting, and UI categories.
New pane-level commands added: `move-pane` (`movep`), `pipe-pane` (`pipep`),
`slice-pane`, `respawn-window` (`respawnw`), `clear-history` (`clearhist`),
and `clear-pane` (`clearp`).
New session/client utility commands:
- `has-session` (`has`): exit code 0 if the named session (`-t`) exists, 1 otherwise — useful in shell scripts.
- `refresh-client` (`refresh`): force a client to redraw (`-d` detach, `-s WxH` resize, `-c pane` select pane, `-D/-U/-L/-R` scroll viewport).
- `suspend-client` (`suspendc`): send SIGTSTP to the dmux client process, returning control to the calling shell.
- `server-access`: manage the server-level access control list (`-a user` allow, `-d user` deny, `-n` deny all new connections, `-w` grant write access).

Option alias commands (for tmux config compatibility):
- `set-window-option` (`setw`): equivalent to `set-option -w`; sets a window-scoped option. Supports `-a` (append), `-F` (format expand), `-o` (only if unset), `-q` (suppress errors), `-u` (unset), `-t target-window`.
- `show-window-options` (`showw`): equivalent to `show-options -w`; lists window-scoped options. Supports `-g` (global window options), `-q`, `-v` (values only), `-t target-window`, and an optional option name to filter output.
- `show-hooks`: lists all hooks registered with `set-hook`, one per line in `hook-name command` format. Supports `-g` and `-t target-session`.
Interactive picker commands `choose-buffer` (`choosebuffer`) and
`choose-client` (`chooseclient`) open a navigable list of paste buffers or
connected clients respectively. Both support `-N` (no preview), `-r` (reverse
order), `-Z` (zoom pane), `-f filter`, `-K key-format`, `-O sort-field`, and
an optional `[template]` positional argument that replaces `%%` with the
selected item's name on confirmation (defaults: `paste-buffer -b '%%'` and
`switch-client -t '%%'`).
`customize-mode` (`customizemode`) opens a full-screen TUI overlay that lets
the user browse and edit all registered options and key bindings. The tree view
groups options by scope (server / session / window) and bindings by key table.
Navigate with Up/Down arrows (or k/j), expand/collapse nodes with Left/Right
(or h/l), press Enter to edit the selected option value or key binding command,
and press q or Escape to close. The `/` key enters a filter bar for quick
searches. Flags: `-Z` (zoom pane), `-F format`, `-f filter`, `-t target-pane`.
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

### Format variables

The following `#{...}` format variables are available for use in format strings
(e.g. `status-left`, `pane-border-format`, `display-message`):

**Pane** (`pane_*`)
`pane_active`, `pane_at_bottom`, `pane_at_left`, `pane_at_right`, `pane_at_top`,
`pane_bottom`, `pane_current_command`, `pane_current_path`, `pane_dead`,
`pane_dead_signal`, `pane_dead_status`, `pane_dead_time`, `pane_format`,
`pane_height`, `pane_id`, `pane_in_mode`, `pane_index`, `pane_input_off`,
`pane_last`, `pane_left`, `pane_marked`, `pane_marked_set`, `pane_mode`,
`pane_path`, `pane_pid`, `pane_pipe`, `pane_right`, `pane_search_string`,
`pane_start_command`, `pane_start_path`, `pane_synchronized`, `pane_tabs`,
`pane_title`, `pane_top`, `pane_tty`, `pane_unseen_changes`, `pane_width`.

The `pane_at_*` variables are `1` when the pane touches the corresponding
window edge and `0` otherwise.

**Window** (`window_*`)
`window_active`, `window_activity`, `window_activity_flag`, `window_bell_flag`,
`window_bigger`, `window_cell_height`, `window_cell_width`, `window_end_flag`,
`window_flags`, `window_format`, `window_height`, `window_id`, `window_index`,
`window_last_flag`, `window_layout`, `window_linked`, `window_linked_sessions`,
`window_linked_sessions_list`, `window_marked_flag`, `window_name`,
`window_offset_x`, `window_offset_y`, `window_panes`, `window_raw_flags`,
`window_silence_flag`, `window_stack_index`, `window_start_flag`,
`window_visible_layout`, `window_width`, `window_zoomed_flag`.

**Session** (`session_*`)
`session_activity`, `session_alerts`, `session_attached`, `session_attached_list`,
`session_created`, `session_format`, `session_group`, `session_group_attached`,
`session_group_attached_list`, `session_group_list`, `session_group_many_attached`,
`session_group_size`, `session_grouped`, `session_id`, `session_last_attached`,
`session_many_attached`, `session_marked`, `session_name`, `session_path`,
`session_stack`, `session_windows`.

**Client** (`client_*`)
`client_activity`, `client_cell_height`, `client_cell_width`, `client_created`,
`client_discarded`, `client_flags`, `client_height`, `client_key_table`,
`client_last_session`, `client_name`, `client_pid`, `client_prefix`,
`client_readonly`, `client_session`, `client_termfeatures`, `client_termname`,
`client_termtype`, `client_tty`, `client_uid`, `client_user`, `client_width`,
`client_written`.

**Server**
`host`, `host_short`, `pid`, `socket_path`, `start_time`, `version`.

The `window-style` and `window-active-style` window options supply default
foreground and background colours for panes. `window-style` is applied to
inactive panes and `window-active-style` to the active pane. Cells that already
carry an explicit colour are not affected; only cells with the terminal-default
colour inherit the window style value. Both options accept the standard tmux
colour syntax (`fg=<colour>,bg=<colour>`) with named ANSI colours, `colourN`
indexed colours, and `#rrggbb` hex values.

**Copy mode** highlights are controlled by four style options:

- `mode-style`: applied to the selection range and cursor cell (uses
  `AttrReverse` by default when no style is explicitly set).
- `copy-mode-match-style`: applied to non-current search match cells after
  a `/`-search or `SetSearch` call.
- `copy-mode-current-match-style`: applied to the active (current) search
  match — the match the cursor is sitting on.
- `copy-mode-mark-style`: applied to all cells on the line pinned with the
  `set-mark` command. The mark is cleared with `clear-mark`.

All four options accept the standard style syntax. Match and mark styles are
applied before selection and cursor highlights so that the selection and cursor
always take visual priority over match/mark colouring.

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
