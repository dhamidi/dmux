// Package prompt implements command-prompt and confirm-before —
// single-line text input dialogs rendered at the bottom of the
// client viewport.
//
// # Boundary
//
// Both dialogs implement [modes.ClientOverlay] and capture keyboard focus.
// They are constructed via [NewCommand] and [NewConfirm].
//
// # Config
//
// [Config] holds all parameters for a command-prompt overlay:
//
//	type Config struct {
//	    Prompt    string                       // label shown before the input field
//	    Template  string                       // %-escape template: %% is replaced by the confirmed input
//	    History   []string                     // initial history entries; oldest first
//	    OnConfirm func(input string)           // called on Enter with the final input
//	    OnCancel  func()                       // called on Escape
//	    Complete  func(partial string) []string // returns tab-completion candidates; nil disables completion
//	}
//
// OnConfirm and OnCancel are plain callbacks; this package does not
// import internal/command. All behaviour is injected at construction time.
//
// # Line Editing
//
// [CommandMode] supports full single-line editing:
//
//   - Character input inserts at the cursor position.
//   - Backspace / Delete remove the character before / at the cursor.
//   - Left / Right move the cursor by one character.
//   - Ctrl-Left / Ctrl-Right move the cursor by one word.
//   - Home / Ctrl-A jump to the beginning of the line.
//   - End / Ctrl-E jump to the end of the line.
//   - Ctrl-K kills from the cursor to the end of the line.
//   - Ctrl-W deletes the word immediately to the left of the cursor.
//
// Line editing is self-contained; no external editor library is used.
//
// # History
//
// History is injected as [Config.History] (oldest first, newest last) and
// is never read from the filesystem. Up/Down arrows navigate through history
// entries; the current unsaved input is preserved and restored when the user
// navigates past the newest entry.
//
// # Tab Completion
//
// [Config.Complete] is called with the current input prefix when Tab is
// pressed. Successive Tab presses cycle through the returned candidates in
// order. A nil Complete field disables completion entirely.
//
// # confirm-before
//
// [NewConfirm] creates a confirm-before dialog. It accepts y / Y / Enter as
// confirmation and any other key as cancellation. Both outcomes close the
// overlay and call the corresponding callback.
//
// # In isolation
//
// The package is testable without a real command system: construct a prompt
// with stub callbacks, drive [CommandMode.Key] calls, and assert on
// [CommandMode.Input] or the returned [modes.Outcome].
//
// # Non-goals
//
// No multi-line editing. Anything needing a real text editor should shell
// out to $EDITOR via a normal pane or popup overlay.
package prompt
