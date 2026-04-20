package session

import (
	"fmt"
	"time"

	"github.com/dhamidi/dmux/internal/format"
)

// PaneContext is a [format.Context] backed by pane metadata.
// It exposes all pane_* format variables defined by dmux.
//
// PaneContext is typically constructed from a Window's pane list by
// [Window.Children] or by the server when expanding pane-scoped format strings.
// All fields are exported so that callers can fill in values that are not
// directly derivable from the session graph (e.g. TTY path, foreground command).
type PaneContext struct {
	// PaneID is the pane's unique identifier within its window.
	PaneID PaneID
	// PaneIndex is the 0-based position of this pane within the window's pane list.
	PaneIndex int

	// Layout geometry — all in character cells.
	Left   int // X offset (column) of the pane's top-left corner.
	Top    int // Y offset (row) of the pane's top-left corner.
	Width  int // Number of columns.
	Height int // Number of rows.

	// WindowWidth and WindowHeight are the total dimensions of the containing
	// window. Used to compute pane_at_* variables.
	WindowWidth  int
	WindowHeight int

	// State flags.
	Active        bool // is this the currently active pane in its window?
	Last          bool // was this the previously active pane?
	Marked        bool // is this pane marked?
	MarkedSet     bool // is any pane in the server currently marked?
	Pipe          bool // is pipe-pane active for this pane?
	InputOff      bool // is keyboard input disabled for this pane?
	InMode        bool // is the pane in copy-mode or another interactive mode?
	Synchronized  bool // are panes in this window synchronized?
	UnseenChanges bool // have there been output changes since this pane was last viewed?
	Zoomed        bool // is this pane the zoomed pane?

	// Mode state.
	Mode         string // name of the current mode ("", "copy-mode", etc.)
	SearchString string // current search string when in copy-mode.
	Tabs         string // tab-stop description string.

	// Process information.
	ShellPID       int    // PID of the shell started inside the pane.
	ForegroundPID  int    // PID of the foreground process (may equal ShellPID).
	TTY            string // PTY device path (e.g. "/dev/pts/3").
	Title          string // pane title as set by the child process via OSC 2.
	CurrentCommand string // command name of the foreground process.
	CurrentPath    string // working directory of the foreground process.
	StartCommand   string // command the pane was originally started with.
	StartPath      string // working directory when the pane was created.

	// Dead-pane information (only meaningful when Dead is true).
	Dead       bool
	DeadStatus int       // exit status of the dead process.
	DeadSignal int       // signal number that killed the process (0 if not signal-killed).
	DeadTime   time.Time // when the process died.
}

// Lookup satisfies [format.Context].
// All pane_* format variables are handled here.
func (pc *PaneContext) Lookup(key string) (string, bool) {
	switch key {
	case "pane_active":
		return boolVal(pc.Active), true
	case "pane_at_bottom":
		return boolVal(pc.Top+pc.Height >= pc.WindowHeight), true
	case "pane_at_left":
		return boolVal(pc.Left == 0), true
	case "pane_at_right":
		return boolVal(pc.Left+pc.Width >= pc.WindowWidth), true
	case "pane_at_top":
		return boolVal(pc.Top == 0), true
	case "pane_bottom":
		return fmt.Sprintf("%d", pc.Top+pc.Height-1), true
	case "pane_current_command":
		return pc.CurrentCommand, true
	case "pane_current_path":
		return pc.CurrentPath, true
	case "pane_dead":
		return boolVal(pc.Dead), true
	case "pane_dead_signal":
		if pc.Dead {
			return fmt.Sprintf("%d", pc.DeadSignal), true
		}
		return "0", true
	case "pane_dead_status":
		if pc.Dead {
			return fmt.Sprintf("%d", pc.DeadStatus), true
		}
		return "0", true
	case "pane_dead_time":
		if pc.Dead && !pc.DeadTime.IsZero() {
			return fmt.Sprintf("%d", pc.DeadTime.Unix()), true
		}
		return "0", true
	case "pane_format":
		return "1", true
	case "pane_height":
		return fmt.Sprintf("%d", pc.Height), true
	case "pane_id":
		return fmt.Sprintf("p%d", pc.PaneID), true
	case "pane_in_mode":
		return boolVal(pc.InMode), true
	case "pane_index":
		return fmt.Sprintf("%d", pc.PaneIndex), true
	case "pane_input_off":
		return boolVal(pc.InputOff), true
	case "pane_last":
		return boolVal(pc.Last), true
	case "pane_left":
		return fmt.Sprintf("%d", pc.Left), true
	case "pane_marked":
		return boolVal(pc.Marked), true
	case "pane_marked_set":
		return boolVal(pc.MarkedSet), true
	case "pane_mode":
		return pc.Mode, true
	case "pane_path":
		if pc.CurrentPath != "" {
			return pc.CurrentPath, true
		}
		return pc.StartPath, true
	case "pane_pid":
		if pc.ForegroundPID != 0 {
			return fmt.Sprintf("%d", pc.ForegroundPID), true
		}
		return fmt.Sprintf("%d", pc.ShellPID), true
	case "pane_pipe":
		return boolVal(pc.Pipe), true
	case "pane_right":
		return fmt.Sprintf("%d", pc.Left+pc.Width-1), true
	case "pane_search_string":
		return pc.SearchString, true
	case "pane_start_command":
		return pc.StartCommand, true
	case "pane_start_path":
		return pc.StartPath, true
	case "pane_synchronized":
		return boolVal(pc.Synchronized), true
	case "pane_tabs":
		return pc.Tabs, true
	case "pane_title":
		return pc.Title, true
	case "pane_top":
		return fmt.Sprintf("%d", pc.Top), true
	case "pane_tty":
		return pc.TTY, true
	case "pane_unseen_changes":
		return boolVal(pc.UnseenChanges), true
	case "pane_width":
		return fmt.Sprintf("%d", pc.Width), true
	}
	return "", false
}

// Children satisfies [format.Context]. PaneContext has no nested contexts.
func (pc *PaneContext) Children(_ string) []format.Context {
	return nil
}
