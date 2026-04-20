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
}
