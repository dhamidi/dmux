package server

import "github.com/dhamidi/dmux/internal/command"

// loadDefaultBindings registers tmux-compatible default key bindings into mut.
// It is called once from Run after s.mutator and s.state.KeyTables are ready.
func loadDefaultBindings(mut command.Mutator) error {
	bindings := []struct{ table, key, cmd string }{
		{"root", "C-b", "switch-key-table prefix"},
		{"prefix", "c", "new-window"},
		{"prefix", "%", "split-window -h"},
		{"prefix", `"`, "split-window"},
		{"prefix", "n", "select-window -t :+"},
		{"prefix", "p", "select-window -t :-"},
		{"prefix", "0", "select-window -t :0"},
		{"prefix", "1", "select-window -t :1"},
		{"prefix", "2", "select-window -t :2"},
		{"prefix", "3", "select-window -t :3"},
		{"prefix", "4", "select-window -t :4"},
		{"prefix", "5", "select-window -t :5"},
		{"prefix", "6", "select-window -t :6"},
		{"prefix", "7", "select-window -t :7"},
		{"prefix", "8", "select-window -t :8"},
		{"prefix", "9", "select-window -t :9"},
		{"prefix", "d", "detach-client"},
		{"prefix", "&", "kill-window"},
		{"prefix", "x", "kill-pane"},
		{"prefix", "?", "list-keys"},
		{"prefix", "[", "copy-mode"},
		{"prefix", "Up", "select-pane -U"},
		{"prefix", "Down", "select-pane -D"},
		{"prefix", "Left", "select-pane -L"},
		{"prefix", "Right", "select-pane -R"},
	}
	for _, b := range bindings {
		if err := mut.BindKey(b.table, b.key, b.cmd); err != nil {
			return err
		}
	}
	return nil
}
