package server

import "github.com/dhamidi/dmux/internal/options"

// loadDefaultOptions registers and sets the built-in default session and
// window options on the server's root option store. It is called once from
// Run before loadDefaultBindings so that all built-in options are available
// from the moment the server starts.
func loadDefaultOptions(s *srv) {
	o := s.state.Options

	// Session options.
	o.Register("mouse", options.Bool, false)
	o.Set("mouse", false) //nolint:errcheck // Register always precedes Set here

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
}
