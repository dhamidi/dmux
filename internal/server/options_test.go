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
		// New session options.
		{"base-index", "0"},
		{"bell-action", "any"},
		{"default-command", ""},
		{"default-size", "80x24"},
		{"destroy-unattached", "off"},
		{"detach-on-destroy", "on"},
		{"display-panes-active-colour", "red"},
		{"display-panes-colour", "blue"},
		{"display-panes-time", "1000"},
		{"display-time", "750"},
		{"history-limit", "2000"},
		{"lock-after-time", "0"},
		{"lock-command", "lock -np"},
		{"message-limit", "100"},
		{"prefix", "C-b"},
		{"prefix2", "None"},
		{"renumber-windows", "off"},
		{"repeat-time", "500"},
		{"set-clipboard", "external"},
		{"set-titles", "off"},
		{"set-titles-string", "#S:#I:#W"},
		{"silence-action", "other"},
		{"status", "on"},
		{"status-interval", "15"},
		{"status-justify", "left"},
		{"status-keys", "emacs"},
		{"status-left", "[#S] "},
		{"status-left-length", "10"},
		{"status-right", " #{session_name}"},
		{"status-right-length", "40"},
		{"update-environment", "DISPLAY SSH_ASKPASS SSH_AUTH_SOCK SSH_AGENT_PID SSH_CONNECTION WINDOWID XAUTHORITY"},
		{"visual-activity", "off"},
		{"visual-bell", "off"},
		{"visual-silence", "off"},
		{"word-separators", " "},
		// New window options.
		{"aggressive-resize", "off"},
		{"allow-passthrough", "off"},
		{"allow-rename", "on"},
		{"alternate-screen", "on"},
		{"clock-mode-colour", "green"},
		{"clock-mode-style", "24"},
		{"cursor-colour", "none"},
		{"cursor-style", "default"},
		{"extended-keys", "off"},
		{"fill-character", ""},
		{"main-pane-height", "24"},
		{"main-pane-width", "80"},
		{"mode-keys", "emacs"},
		{"other-pane-height", "0"},
		{"other-pane-width", "0"},
		{"pane-base-index", "0"},
		{"remain-on-exit", "off"},
		{"remain-on-exit-format", "Pane is dead (#{?pane_dead_status,status #{pane_dead_status},}#{?#{!=:#{pane_dead_signal},},signal #{pane_dead_signal},})"},
		{"scroll-on-output", "off"},
		{"window-size", "latest"},
		{"window-status-current-format", "#I:#W#F"},
		{"window-status-format", "#I:#W#F"},
		{"wrap-search", "on"},
		{"xterm-keys", "on"},
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
