package server

import (
	"os"

	"github.com/dhamidi/dmux/internal/options"
)

// loadDefaultOptions registers and sets the built-in default session and
// window options on the server's root option store. It is called once from
// Run before loadDefaultBindings so that all built-in options are available
// from the moment the server starts.
func loadDefaultOptions(s *srv) {
	o := s.state.Options

	// Session options.
	o.Register("mouse", options.Bool, false)
	o.Set("mouse", false) //nolint:errcheck // Register always precedes Set here

	o.Register("base-index", options.Int, 0)
	o.Set("base-index", 0) //nolint:errcheck

	o.Register("bell-action", options.String, "any")
	o.Set("bell-action", "any") //nolint:errcheck

	o.Register("default-command", options.String, "")
	o.Set("default-command", "") //nolint:errcheck

	defaultShell := os.Getenv("SHELL")
	o.Register("default-shell", options.String, defaultShell)
	o.Set("default-shell", defaultShell) //nolint:errcheck

	o.Register("default-size", options.String, "80x24")
	o.Set("default-size", "80x24") //nolint:errcheck

	o.Register("destroy-unattached", options.Bool, false)
	o.Set("destroy-unattached", false) //nolint:errcheck

	o.Register("detach-on-destroy", options.String, "on")
	o.Set("detach-on-destroy", "on") //nolint:errcheck

	o.Register("display-panes-active-colour", options.String, "red")
	o.Set("display-panes-active-colour", "red") //nolint:errcheck

	o.Register("display-panes-colour", options.String, "blue")
	o.Set("display-panes-colour", "blue") //nolint:errcheck

	o.Register("display-panes-time", options.Int, 1000)
	o.Set("display-panes-time", 1000) //nolint:errcheck

	o.Register("display-time", options.Int, 750)
	o.Set("display-time", 750) //nolint:errcheck

	o.Register("history-limit", options.Int, 2000)
	o.Set("history-limit", 2000) //nolint:errcheck

	o.Register("lock-after-time", options.Int, 0)
	o.Set("lock-after-time", 0) //nolint:errcheck

	o.Register("lock-command", options.String, "lock -np")
	o.Set("lock-command", "lock -np") //nolint:errcheck

	o.Register("message-limit", options.Int, 100)
	o.Set("message-limit", 100) //nolint:errcheck

	o.Register("prefix", options.String, "C-b")
	o.Set("prefix", "C-b") //nolint:errcheck

	o.Register("prefix2", options.String, "None")
	o.Set("prefix2", "None") //nolint:errcheck

	o.Register("renumber-windows", options.Bool, false)
	o.Set("renumber-windows", false) //nolint:errcheck

	o.Register("repeat-time", options.Int, 500)
	o.Set("repeat-time", 500) //nolint:errcheck

	o.Register("set-clipboard", options.String, "external")
	o.Set("set-clipboard", "external") //nolint:errcheck

	o.Register("set-titles", options.Bool, false)
	o.Set("set-titles", false) //nolint:errcheck

	o.Register("set-titles-string", options.String, "#S:#I:#W")
	o.Set("set-titles-string", "#S:#I:#W") //nolint:errcheck

	o.Register("silence-action", options.String, "other")
	o.Set("silence-action", "other") //nolint:errcheck

	o.Register("status", options.String, "on")
	o.Set("status", "on") //nolint:errcheck

	o.Register("status-interval", options.Int, 15)
	o.Set("status-interval", 15) //nolint:errcheck

	o.Register("status-justify", options.String, "left")
	o.Set("status-justify", "left") //nolint:errcheck

	o.Register("status-keys", options.String, "emacs")
	o.Set("status-keys", "emacs") //nolint:errcheck

	o.Register("status-left", options.String, "[#S] ")
	o.Set("status-left", "[#S] ") //nolint:errcheck

	o.Register("status-left-length", options.Int, 10)
	o.Set("status-left-length", 10) //nolint:errcheck

	o.Register("status-right", options.String, " #{session_name}")
	o.Set("status-right", " #{session_name}") //nolint:errcheck

	o.Register("status-right-length", options.Int, 40)
	o.Set("status-right-length", 40) //nolint:errcheck

	o.Register("update-environment", options.String, "DISPLAY SSH_ASKPASS SSH_AUTH_SOCK SSH_AGENT_PID SSH_CONNECTION WINDOWID XAUTHORITY")
	o.Set("update-environment", "DISPLAY SSH_ASKPASS SSH_AUTH_SOCK SSH_AGENT_PID SSH_CONNECTION WINDOWID XAUTHORITY") //nolint:errcheck

	o.Register("visual-activity", options.String, "off")
	o.Set("visual-activity", "off") //nolint:errcheck

	o.Register("visual-bell", options.String, "off")
	o.Set("visual-bell", "off") //nolint:errcheck

	o.Register("visual-silence", options.String, "off")
	o.Set("visual-silence", "off") //nolint:errcheck

	o.Register("word-separators", options.String, " ")
	o.Set("word-separators", " ") //nolint:errcheck

	// Window options.
	o.Register("automatic-rename", options.Bool, true)
	o.Set("automatic-rename", true) //nolint:errcheck

	o.Register("automatic-rename-format", options.String, "#{?pane_in_mode,[tmux],#{pane_current_command}}")
	o.Set("automatic-rename-format", "#{?pane_in_mode,[tmux],#{pane_current_command}}") //nolint:errcheck

	o.Register("monitor-activity", options.Bool, false)
	o.Set("monitor-activity", false) //nolint:errcheck

	o.Register("monitor-bell", options.Bool, false)
	o.Set("monitor-bell", false) //nolint:errcheck

	o.Register("monitor-silence", options.Int, 0)
	o.Set("monitor-silence", 0) //nolint:errcheck

	o.Register("synchronize-panes", options.Bool, false)
	o.Set("synchronize-panes", false) //nolint:errcheck

	// Server-scope style options.
	o.Register("message-style", options.Style, "bg=yellow,fg=black")
	o.Set("message-style", "bg=yellow,fg=black") //nolint:errcheck
	o.Register("message-command-style", options.Style, "bg=black,fg=yellow")
	o.Set("message-command-style", "bg=black,fg=yellow") //nolint:errcheck

	// Session-scope style options.
	o.Register("status-style", options.Style, "bg=green,fg=black")
	o.Set("status-style", "bg=green,fg=black") //nolint:errcheck
	o.Register("status-left-style", options.Style, "default")
	o.Set("status-left-style", "default") //nolint:errcheck
	o.Register("status-right-style", options.Style, "default")
	o.Set("status-right-style", "default") //nolint:errcheck
	o.Register("window-status-style", options.Style, "default")
	o.Set("window-status-style", "default") //nolint:errcheck
	o.Register("window-status-current-style", options.Style, "reverse")
	o.Set("window-status-current-style", "reverse") //nolint:errcheck
	o.Register("window-status-activity-style", options.Style, "reverse")
	o.Set("window-status-activity-style", "reverse") //nolint:errcheck
	o.Register("window-status-bell-style", options.Style, "default,alert")
	o.Set("window-status-bell-style", "default,alert") //nolint:errcheck
	o.Register("window-status-last-style", options.Style, "default")
	o.Set("window-status-last-style", "default") //nolint:errcheck
	o.Register("window-status-separator", options.String, " ")
	o.Set("window-status-separator", " ") //nolint:errcheck
	o.Register("mode-style", options.Style, "bg=yellow,fg=black")
	o.Set("mode-style", "bg=yellow,fg=black") //nolint:errcheck
	o.Register("copy-mode-match-style", options.Style, "bg=cyan,fg=black")
	o.Set("copy-mode-match-style", "bg=cyan,fg=black") //nolint:errcheck
	o.Register("copy-mode-current-match-style", options.Style, "bg=orange,fg=black")
	o.Set("copy-mode-current-match-style", "bg=orange,fg=black") //nolint:errcheck
	o.Register("copy-mode-mark-style", options.Style, "reverse")
	o.Set("copy-mode-mark-style", "reverse") //nolint:errcheck
	o.Register("popup-style", options.Style, "default")
	o.Set("popup-style", "default") //nolint:errcheck
	o.Register("popup-border-style", options.Style, "default")
	o.Set("popup-border-style", "default") //nolint:errcheck
	o.Register("popup-border-lines", options.String, "single")
	o.Set("popup-border-lines", "single") //nolint:errcheck
	o.Register("menu-style", options.Style, "default")
	o.Set("menu-style", "default") //nolint:errcheck
	o.Register("menu-border-style", options.Style, "default")
	o.Set("menu-border-style", "default") //nolint:errcheck
	o.Register("menu-selected-style", options.Style, "reverse")
	o.Set("menu-selected-style", "reverse") //nolint:errcheck

	// Window-scope style options.
	o.Register("window-style", options.Style, "default")
	o.Set("window-style", "default") //nolint:errcheck
	o.Register("window-active-style", options.Style, "default")
	o.Set("window-active-style", "default") //nolint:errcheck
	o.Register("pane-border-style", options.Style, "default")
	o.Set("pane-border-style", "default") //nolint:errcheck
	o.Register("pane-active-border-style", options.Style, "fg=green")
	o.Set("pane-active-border-style", "fg=green") //nolint:errcheck
	o.Register("pane-border-format", options.String, "")
	o.Set("pane-border-format", "") //nolint:errcheck
	o.Register("pane-border-status", options.String, "off")
	o.Set("pane-border-status", "off") //nolint:errcheck
	o.Register("pane-border-lines", options.String, "single")
	o.Set("pane-border-lines", "single") //nolint:errcheck

	// Additional window options.
	o.Register("aggressive-resize", options.Bool, false)
	o.Set("aggressive-resize", false) //nolint:errcheck

	o.Register("allow-passthrough", options.String, "off")
	o.Set("allow-passthrough", "off") //nolint:errcheck

	o.Register("allow-rename", options.Bool, true)
	o.Set("allow-rename", true) //nolint:errcheck

	o.Register("alternate-screen", options.Bool, true)
	o.Set("alternate-screen", true) //nolint:errcheck

	o.Register("clock-mode-colour", options.String, "green")
	o.Set("clock-mode-colour", "green") //nolint:errcheck

	o.Register("clock-mode-style", options.String, "24")
	o.Set("clock-mode-style", "24") //nolint:errcheck

	o.Register("cursor-colour", options.String, "none")
	o.Set("cursor-colour", "none") //nolint:errcheck

	o.Register("cursor-style", options.String, "default")
	o.Set("cursor-style", "default") //nolint:errcheck

	o.Register("extended-keys", options.String, "off")
	o.Set("extended-keys", "off") //nolint:errcheck

	o.Register("fill-character", options.String, "")
	o.Set("fill-character", "") //nolint:errcheck

	o.Register("main-pane-height", options.Int, 24)
	o.Set("main-pane-height", 24) //nolint:errcheck

	o.Register("main-pane-width", options.Int, 80)
	o.Set("main-pane-width", 80) //nolint:errcheck

	o.Register("mode-keys", options.String, "emacs")
	o.Set("mode-keys", "emacs") //nolint:errcheck

	o.Register("other-pane-height", options.Int, 0)
	o.Set("other-pane-height", 0) //nolint:errcheck

	o.Register("other-pane-width", options.Int, 0)
	o.Set("other-pane-width", 0) //nolint:errcheck

	o.Register("pane-base-index", options.Int, 0)
	o.Set("pane-base-index", 0) //nolint:errcheck

	o.Register("remain-on-exit", options.String, "off")
	o.Set("remain-on-exit", "off") //nolint:errcheck

	o.Register("remain-on-exit-format", options.String, "Pane is dead (#{?pane_dead_status,status #{pane_dead_status},}#{?#{!=:#{pane_dead_signal},},signal #{pane_dead_signal},})")
	o.Set("remain-on-exit-format", "Pane is dead (#{?pane_dead_status,status #{pane_dead_status},}#{?#{!=:#{pane_dead_signal},},signal #{pane_dead_signal},})") //nolint:errcheck

	o.Register("scroll-on-output", options.Bool, false)
	o.Set("scroll-on-output", false) //nolint:errcheck

	o.Register("window-size", options.String, "latest")
	o.Set("window-size", "latest") //nolint:errcheck

	o.Register("window-status-current-format", options.String, "#I:#W#F")
	o.Set("window-status-current-format", "#I:#W#F") //nolint:errcheck

	o.Register("window-status-format", options.String, "#I:#W#F")
	o.Set("window-status-format", "#I:#W#F") //nolint:errcheck

	o.Register("wrap-search", options.Bool, true)
	o.Set("wrap-search", true) //nolint:errcheck

	o.Register("xterm-keys", options.Bool, true)
	o.Set("xterm-keys", true) //nolint:errcheck
}
