// Package newwindow implements the new-window command.
//
// # Synopsis
//
//	new-window
//
// # Typed args
//
//	type Args struct {
//	    AfterCurrent  bool   `dmux:"a"           help:"insert after the current window"`
//	    BeforeCurrent bool   `dmux:"b"           help:"insert before the current window"`
//	    Cwd           string `dmux:"c=dir"       help:"start the pane in dir"`
//	    Detached      bool   `dmux:"d"           help:"do not switch to the new window"`
//	    Format        string `dmux:"F=format"    help:"print-after-creation format"`
//	    Kill          bool   `dmux:"k"           help:"destroy a window at target if it exists"`
//	    WindowName    string `dmux:"n=name"      help:"new window name"`
//	    Print         bool   `dmux:"P"           help:"print information about the new window"`
//	    SelectSession bool   `dmux:"S"           help:"select the existing window if present"`
//	    Target        string `dmux:"t=target"    help:"target window or session"`
//	    ShellCommand  string `dmux:""            help:"shell command to run in the new pane"`
//	}
//
// Milestone one implements no flags: the command takes no arguments
// and always appends a new window to the caller's current session
// using the server's default shell.
//
// Deferred (not on the struct yet): AfterCurrent, BeforeCurrent,
// Cwd, Detached, Format, Kill, WindowName, Print, SelectSession,
// Target, ShellCommand.
//
// # Behavior
//
//  1. Read the caller's current session off the Item. A nil session
//     means the connection is not attached; return
//     cmd.Err(ErrNotFound).
//  2. Ask the server to spawn a new window in the session via
//     SpawnWindow with an empty name (empty-name semantics: the
//     server picks the default, typically the shell basename).
//  3. Return cmd.Ok. AppendWindow inside the server sets the new
//     window as the session's current window, so the next render
//     tick will show it.
//
// # Error sentinels
//
// The M1 no-arg case uses cmd.ErrNotFound when CurrentSession is
// nil. Server-level failures (pane spawn, pty allocation) come back
// wrapped by the server's SpawnWindow implementation.
//
// # Registration
//
//	const Name = "new-window"
//
//	type command struct{}
//
//	func (command) Name() string { return Name }
//	func (command) Exec(item cmd.Item, _ []string) cmd.Result { ... }
//
//	func init() { cmd.Register(command{}) }
//
// # Corresponding tmux code
//
// cmd-new-window.c.
package newwindow
