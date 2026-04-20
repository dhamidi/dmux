package builtin

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "refresh-client",
		Alias: []string{"refresh"},
		Args: command.ArgSpec{
			Flags:   []string{"D", "U", "L", "R", "d", "l"},
			Options: []string{"c", "f", "s", "t", "B"},
		},
		Run: runRefreshClient,
	})
}

// runRefreshClient forces a client to redraw or adjusts client-specific
// settings. Without any flags it triggers a full redraw of the target client.
//
// Flags:
//   - -t target-client: the client to act on (defaults to calling client).
//   - -d: detach the client.
//   - -c target-pane: set the active pane for the client.
//   - -s WxH: set client size.
//   - -D/-U/-L/-R: scroll the client viewport (stub — records the direction).
//   - -f flags: set client feature flags (stub).
//   - -l: request clipboard content via OSC 52 (stub).
//   - -B name:notify:format: subscribe to a named notification (stub).
func runRefreshClient(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("refresh-client: no mutator available")
	}

	clientID := ctx.Args.Option("t")
	if clientID == "" {
		clientID = ctx.Client.ID
	}

	// -d: detach the client.
	if ctx.Args.Flag("d") {
		if err := ctx.Mutator.DetachClient(clientID); err != nil {
			return command.Errorf("refresh-client: %v", err)
		}
		return command.OK()
	}

	// -c target-pane: select the active pane for this client.
	if paneTarget := ctx.Args.Option("c"); paneTarget != "" {
		// Re-use select-pane logic: look up the target pane in the client's
		// current session and select it.
		client, ok := ctx.Server.GetClient(clientID)
		if !ok {
			return command.Errorf("refresh-client: client %q not found", clientID)
		}
		sess, ok := ctx.Server.GetSession(client.SessionID)
		if !ok {
			return command.Errorf("refresh-client: client %q is not attached to a session", clientID)
		}
		win := sess.CurrentWindow()
		for _, p := range win.Panes {
			if fmt.Sprintf("%%%d", p.ID) == paneTarget || strconv.Itoa(p.ID) == paneTarget {
				if err := ctx.Mutator.SelectPane(sess.ID, win.ID, p.ID); err != nil {
					return command.Errorf("refresh-client: %v", err)
				}
				return command.OK()
			}
		}
		return command.Errorf("refresh-client: pane %q not found", paneTarget)
	}

	// -s WxH: resize the client.
	if sizeStr := ctx.Args.Option("s"); sizeStr != "" {
		parts := strings.SplitN(sizeStr, "x", 2)
		if len(parts) != 2 {
			return command.Errorf("refresh-client: invalid size %q, expected WxH", sizeStr)
		}
		cols, err1 := strconv.Atoi(parts[0])
		rows, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return command.Errorf("refresh-client: invalid size %q, expected WxH", sizeStr)
		}
		if err := ctx.Mutator.ResizeClient(clientID, cols, rows); err != nil {
			return command.Errorf("refresh-client: %v", err)
		}
		return command.OK()
	}

	// -f flags: set client feature flags (stub).
	// -l: request clipboard via OSC 52 (stub).
	// -B name:notify:format: subscribe to notification (stub).
	// -D/-U/-L/-R: scroll the client viewport (stub).
	// These are accepted but not yet enforced; a full redraw is triggered.

	// Default (and fallback): send a full redraw signal to the client.
	if err := ctx.Mutator.RefreshClient(clientID); err != nil {
		return command.Errorf("refresh-client: %v", err)
	}
	return command.OK()
}
