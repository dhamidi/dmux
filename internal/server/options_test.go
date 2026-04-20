package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/session"
)

func TestLoadDefaultOptions(t *testing.T) {
	state := session.NewServer()
	s := &srv{state: state}
	loadDefaultOptions(s)

	mut := newServerMutator(state, nil, nil, func() {},
		func(session.ClientID) (*clientConn, bool) { return nil, false },
		func(*clientConn) {},
		nil, nil,
	)

	entries := mut.ListOptions("server")
	byName := make(map[string]string, len(entries))
	for _, e := range entries {
		byName[e.Name] = e.Value
	}

	checks := []struct {
		name string
		want string
	}{
		{"mouse", "off"},
		{"automatic-rename", "on"},
		{"automatic-rename-format", "#{?pane_in_mode,[tmux],#{pane_current_command}}"},
		{"monitor-activity", "off"},
		{"monitor-bell", "off"},
		{"monitor-silence", "0"},
		{"synchronize-panes", "off"},
		// Server-scope style options.
		{"message-style", "bg=yellow,fg=black"},
		{"message-command-style", "bg=black,fg=yellow"},
		// Session-scope style options.
		{"status-style", "bg=green,fg=black"},
		{"status-left-style", "default"},
		{"status-right-style", "default"},
		{"window-status-style", "default"},
		{"window-status-current-style", "reverse"},
		{"window-status-activity-style", "reverse"},
		{"window-status-bell-style", "default,alert"},
		{"window-status-last-style", "default"},
		{"window-status-separator", " "},
		{"mode-style", "bg=yellow,fg=black"},
		{"copy-mode-match-style", "bg=cyan,fg=black"},
		{"copy-mode-current-match-style", "bg=orange,fg=black"},
		{"copy-mode-mark-style", "reverse"},
		{"popup-style", "default"},
		{"popup-border-style", "default"},
		{"popup-border-lines", "single"},
		{"menu-style", "default"},
		{"menu-border-style", "default"},
		{"menu-selected-style", "reverse"},
		// Window-scope style options.
		{"window-style", "default"},
		{"window-active-style", "default"},
		{"pane-border-style", "default"},
		{"pane-active-border-style", "fg=green"},
		{"pane-border-format", ""},
		{"pane-border-status", "off"},
		{"pane-border-lines", "single"},
	}
	for _, c := range checks {
		got, ok := byName[c.name]
		if !ok {
			t.Errorf("option %q not present in ListOptions(\"server\")", c.name)
			continue
		}
		if got != c.want {
			t.Errorf("option %q = %q, want %q", c.name, got, c.want)
		}
	}
}
