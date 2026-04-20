package session_test

import (
	"fmt"
	"testing"

	"github.com/dhamidi/dmux/internal/format"
	"github.com/dhamidi/dmux/internal/session"
)

// fullPaneContext returns a PaneContext with all fields populated with
// non-zero test values, suitable for verifying that every pane_* variable
// produces a non-empty string.
func fullPaneContext() *session.PaneContext {
	return &session.PaneContext{
		PaneID:         1,
		PaneIndex:      0,
		Left:           0,
		Top:            0,
		Width:          80,
		Height:         24,
		WindowWidth:    80,
		WindowHeight:   24,
		Active:         true,
		Last:           false,
		Title:          "bash",
		ShellPID:       1234,
		TTY:            "/dev/pts/3",
		CurrentCommand: "vim",
		CurrentPath:    "/home/user/project",
		StartCommand:   "/bin/bash",
		StartPath:      "/home/user",
		Mode:           "copy-mode",
		SearchString:   "hello",
		Tabs:           "8,16,24,32",
	}
}

// TestPaneContextAllVariablesNonEmpty verifies that every pane_* format variable
// expands to a non-empty string when the PaneContext is fully populated.
func TestPaneContextAllVariablesNonEmpty(t *testing.T) {
	vars := []string{
		"pane_active",
		"pane_at_bottom",
		"pane_at_left",
		"pane_at_right",
		"pane_at_top",
		"pane_bottom",
		"pane_current_command",
		"pane_current_path",
		"pane_dead",
		"pane_dead_signal",
		"pane_dead_status",
		"pane_dead_time",
		"pane_format",
		"pane_height",
		"pane_id",
		"pane_in_mode",
		"pane_index",
		"pane_input_off",
		"pane_last",
		"pane_left",
		"pane_marked",
		"pane_marked_set",
		"pane_mode",
		"pane_path",
		"pane_pid",
		"pane_pipe",
		"pane_right",
		"pane_search_string",
		"pane_start_command",
		"pane_start_path",
		"pane_synchronized",
		"pane_tabs",
		"pane_title",
		"pane_top",
		"pane_tty",
		"pane_unseen_changes",
		"pane_width",
	}

	pc := fullPaneContext()

	for _, v := range vars {
		t.Run(v, func(t *testing.T) {
			got, err := format.Expand("#{"+v+"}", pc)
			if err != nil {
				t.Fatalf("Expand #{%s}: unexpected error: %v", v, err)
			}
			if got == "" {
				t.Fatalf("Expand #{%s}: got empty string, want non-empty", v)
			}
		})
	}
}

// TestPaneAtVariablesPosition verifies the computed pane_at_* boolean variables
// for panes at various positions within a window.
func TestPaneAtVariablesPosition(t *testing.T) {
	tests := []struct {
		name                                    string
		left, top, width, height, winW, winH   int
		wantTop, wantBottom, wantLeft, wantRight string
	}{
		{
			name:        "full-window single pane",
			left: 0, top: 0, width: 80, height: 24, winW: 80, winH: 24,
			wantTop: "1", wantBottom: "1", wantLeft: "1", wantRight: "1",
		},
		{
			name:        "top-left pane in 2x2 split",
			left: 0, top: 0, width: 40, height: 12, winW: 80, winH: 24,
			wantTop: "1", wantBottom: "0", wantLeft: "1", wantRight: "0",
		},
		{
			name:        "top-right pane in 2x2 split",
			left: 40, top: 0, width: 40, height: 12, winW: 80, winH: 24,
			wantTop: "1", wantBottom: "0", wantLeft: "0", wantRight: "1",
		},
		{
			name:        "bottom-left pane in 2x2 split",
			left: 0, top: 12, width: 40, height: 12, winW: 80, winH: 24,
			wantTop: "0", wantBottom: "1", wantLeft: "1", wantRight: "0",
		},
		{
			name:        "bottom-right pane in 2x2 split",
			left: 40, top: 12, width: 40, height: 12, winW: 80, winH: 24,
			wantTop: "0", wantBottom: "1", wantLeft: "0", wantRight: "1",
		},
		{
			name:        "left pane in vertical split",
			left: 0, top: 0, width: 40, height: 24, winW: 80, winH: 24,
			wantTop: "1", wantBottom: "1", wantLeft: "1", wantRight: "0",
		},
		{
			name:        "right pane in vertical split",
			left: 40, top: 0, width: 40, height: 24, winW: 80, winH: 24,
			wantTop: "1", wantBottom: "1", wantLeft: "0", wantRight: "1",
		},
		{
			name:        "top pane in horizontal split",
			left: 0, top: 0, width: 80, height: 12, winW: 80, winH: 24,
			wantTop: "1", wantBottom: "0", wantLeft: "1", wantRight: "1",
		},
		{
			name:        "bottom pane in horizontal split",
			left: 0, top: 12, width: 80, height: 12, winW: 80, winH: 24,
			wantTop: "0", wantBottom: "1", wantLeft: "1", wantRight: "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := &session.PaneContext{
				Left:         tt.left,
				Top:          tt.top,
				Width:        tt.width,
				Height:       tt.height,
				WindowWidth:  tt.winW,
				WindowHeight: tt.winH,
			}
			check := func(varName, want string) {
				t.Helper()
				got, err := format.Expand("#{"+varName+"}", pc)
				if err != nil {
					t.Errorf("#{%s}: unexpected error: %v", varName, err)
					return
				}
				if got != want {
					t.Errorf("#{%s}: got %q, want %q", varName, got, want)
				}
			}
			check("pane_at_top", tt.wantTop)
			check("pane_at_bottom", tt.wantBottom)
			check("pane_at_left", tt.wantLeft)
			check("pane_at_right", tt.wantRight)
		})
	}
}

// TestPaneContextGeometry verifies that pane_left, pane_top, pane_right,
// pane_bottom are computed correctly from the layout coordinates.
func TestPaneContextGeometry(t *testing.T) {
	pc := &session.PaneContext{
		Left: 10, Top: 5, Width: 40, Height: 20,
	}
	checkInt := func(varName string, want int) {
		t.Helper()
		got, err := format.Expand("#{"+varName+"}", pc)
		if err != nil {
			t.Fatalf("#{%s}: unexpected error: %v", varName, err)
		}
		if got != fmt.Sprintf("%d", want) {
			t.Errorf("#{%s}: got %q, want %d", varName, got, want)
		}
	}
	checkInt("pane_left", 10)
	checkInt("pane_top", 5)
	checkInt("pane_right", 49)  // 10 + 40 - 1 = 49
	checkInt("pane_bottom", 24) // 5 + 20 - 1 = 24
	checkInt("pane_width", 40)
	checkInt("pane_height", 20)
}
