# Modes and overlays

Modal UI in dmux splits into two shapes. Getting this split right is
load-bearing — everything from `prefix q` to nested key bindings falls
out of it.

## PaneMode

A `PaneMode` fills one pane's rectangle and takes over that pane's key
handling. The pane's underlying shell keeps running in the background:
output continues being parsed into the libghostty Terminal, it just
isn't shown. On exit, any new output is there.

Implementers:

- `modes/copy` — vi/emacs-style scrollback editor
- `modes/tree` — `choose-tree`, `choose-session`, `choose-window`
- (future) `modes/clock` — `clock-mode`

Entering a PaneMode also switches the client's `KeyTable` to the mode's
table name (`copy-mode-vi`, `copy-mode`, etc.). This is why `j` and
`/` do different things in copy-mode than in the shell — not because
the mode intercepts them, but because a different key table routes
them to different bindings.

## ClientOverlay

A `ClientOverlay` is drawn over the composed window frame in client
(screen) coordinates. It may or may not capture focus. Implementers:

- `modes/popup` — `display-popup`, a floating pane
- `modes/menu` — `display-menu`, tmux-style popup menu
- `modes/displaypanes` — `prefix q`, big numerals over each pane
- `modes/prompt` — `command-prompt`, `confirm-before`

Overlays stack on the client. The top overlay that captures focus
receives keys; overlays that don't capture focus (like the
display-panes numerals during their brief visible window) just render.

## Why the split matters

Both concepts look similar on the surface ("modal UI") but they plug
into different parts of the system. PaneModes change what a pane
displays and where its keys go. ClientOverlays change what's drawn on
top of everything. Forcing them to share an interface would add a
"Rect means what here?" conditional to every mode and a "which code
path even runs me?" conditional to render and the server loop.

## Key-table mechanics

Each attached client has a `KeyTable` string field. The server loop's
key dispatch path:

1. Decode byte stream from client into a `keys.Key`.
2. Look up `keys.Registry[client.KeyTable]`.
3. Find the binding for the key in that table.
4. Enqueue the binding's command on the client's command queue.
5. If the binding isn't marked as persistent, reset `KeyTable` to
   `"root"` after it fires.

This is how `prefix` tables work: `bind-key -T prefix c new-window`
means "when the key `c` is pressed and the client's KeyTable is
`prefix`, run `new-window`, then go back to `root`." Modes use the
same mechanism to install their own tables.
