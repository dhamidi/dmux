// Package newsession implements the new-session command.
//
// # Synopsis
//
// A subset of tmux's cmd-new-session.c:
//
//	new-session [-Ad] [-s session-name] [-n window-name]
//	            [-x width] [-y height] [-c start-directory]
//	            [shell-command]
//
// # Typed args
//
// The command's argument struct, parsed via the args sub-package
// from the struct tags. See `internal/cmd/doc.go` and
// `docs/go-patterns.md` for the tag dialect.
//
//	type Args struct {
//	    Detached    bool     `dmux:"d"        help:"don't attach client to session"`
//	    Attach      bool     `dmux:"A"        help:"attach if session exists"`
//	    Name        string   `dmux:"s=name"   help:"session name"`
//	    WindowName  string   `dmux:"n=name"   help:"first window name"`
//	    StartDir    string   `dmux:"c=path"   help:"start directory"`
//	    Width       int      `dmux:"x=cols"   help:"initial cell width"`
//	    Height      int      `dmux:"y=rows"   help:"initial cell height"`
//	    Command     []string `dmux:"$"        help:"shell command"`
//	}
//
// Flags implemented in milestone one: Detached, Name, WindowName,
// StartDir.
//
// Deferred (declared on the struct so unknown-flag errors stay
// accurate, but rejected with "not implemented yet" at runtime):
// Attach, Width, Height. The unlisted -E/-F/-P/-t flags are not on
// the struct and parse as unknown flags until their slices land.
//
// # Behavior for the milestone-one path (plain "dmux")
//
//  1. Resolve the session name: a.Name, or an auto-generated integer
//     mirroring tmux's "lowest unused integer" scheme.
//  2. Resolve cwd: a.StartDir, or the attached client's cwd, or
//     $HOME.
//  3. Resolve shell: session option default-shell, or $SHELL, or the
//     user's passwd shell, or /bin/sh (%COMSPEC% on Windows).
//  4. Create a Window with one Pane running the shell via pty.Spawn
//     (see internal/pty and internal/pane). The pane reader goroutine
//     is bound to the new pane's context, which is a child of the
//     session's context, which is a child of the server's root.
//  5. Create a Session containing one Winlink pointing at the window,
//     registered in the server's session.Registry.
//  6. If not a.Detached, set this session as the invoking client's
//     current session and tail-chain into attach-session via
//     cmd.Run, so the next tick of the main loop attaches.
//
// # Registration
//
//	func init() {
//	    cmd.Register(cmd.New("new-session", []string{"new"}, exec))
//	}
//
//	func exec(item *cmd.Item, a *Args) cmd.Result {
//	    // ... resolve, spawn, register ...
//	    if a.Detached {
//	        return cmd.Ok
//	    }
//	    return cmd.Run(attachsession.Cmd, "-t", sessionName)
//	}
//
// # Scope boundary
//
// No environment override, hook firing, or layout logic beyond what
// is required to get a single pane running the user's shell. Grouped
// sessions, session templates, and the -E / -F / -P flags are future
// work.
//
// # Corresponding tmux code
//
// cmd-new-session.c.
package newsession
