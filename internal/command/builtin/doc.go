// Package builtin contains gomux's built-in commands, one file per
// command.
//
// # Boundary
//
// Each file defines a command.Spec and registers it in init(). The
// package exports nothing — its entire effect is the side effect of
// import-time registration.
//
//	import _ "github.com/yourname/gomux/internal/command/builtin"
//
// Approximately ~60 commands total, grouped by file naming convention
// that mirrors tmux's:
//
//   - session management:  new_session.go, kill_session.go,
//                          attach_session.go, detach_client.go,
//                          switch_client.go, rename_session.go, ...
//   - window management:   new_window.go, kill_window.go,
//                          select_window.go, rename_window.go,
//                          move_window.go, swap_window.go,
//                          find_window.go, ...
//   - pane management:     split_window.go, kill_pane.go,
//                          select_pane.go, resize_pane.go,
//                          swap_pane.go, break_pane.go, join_pane.go,
//                          pipe_pane.go, capture_pane.go,
//                          respawn_pane.go, ...
//   - key bindings:        bind_key.go, unbind_key.go, send_keys.go
//   - options:             set_option.go, show_options.go,
//                          set_environment.go, show_environment.go
//   - buffers:             set_buffer.go, load_buffer.go,
//                          save_buffer.go, paste_buffer.go,
//                          list_buffers.go, delete_buffer.go
//   - modes / UI:          copy_mode.go, choose_tree.go,
//                          display_menu.go, display_popup.go,
//                          display_panes.go, display_message.go,
//                          command_prompt.go, confirm_before.go,
//                          clock_mode.go
//   - scripting:           run_shell.go, if_shell.go, wait_for.go,
//                          source_file.go
//   - lists:               list_sessions.go, list_windows.go,
//                          list_panes.go, list_clients.go,
//                          list_keys.go, list_commands.go
//   - server:              kill_server.go, start_server.go,
//                          show_messages.go, lock_server.go
//
// # Splitting for embedders
//
// Someone embedding gomux who doesn't want copy-mode or menus can
// import only the sub-sets they need. The framework (package command)
// has no hard requirement on any specific builtin being present.
//
// # Non-goals
//
// No shared helpers between commands unless they naturally belong
// somewhere else — shared target resolution lives in package command,
// shared option lookups live in package options, and so on. Each
// builtin is a thin translation from ParsedArgs to Server mutations.
package builtin
