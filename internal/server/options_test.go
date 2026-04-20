package server

import (
	"testing"

	"github.com/dhamidi/dmux/internal/session"
)

func TestLoadDefaultOptions(t *testing.T) {
	state := session.NewServer()
	s := &srv{state: state}
	loadDefaultOptions(s)

	mut := newServerMutator(state, func() {},
		func(session.ClientID) (*clientConn, bool) { return nil, false },
		func(*clientConn) {},
		nil,
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
