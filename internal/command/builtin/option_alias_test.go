package builtin_test

import (
	"strings"
	"testing"

	"github.com/dhamidi/dmux/internal/command"
)

// TestSetWindowOption_SetsWindowScope verifies that set-window-option sets the
// option at window scope.
func TestSetWindowOption_SetsWindowScope(t *testing.T) {
	b := newBackend()
	res := dispatch("set-window-option", []string{"automatic-rename", "off"}, b)
	if res.Err != nil {
		t.Fatalf("set-window-option returned error: %v", res.Err)
	}
	if len(b.setOptions) != 1 {
		t.Fatalf("expected 1 SetOption call, got %d", len(b.setOptions))
	}
	got := b.setOptions[0]
	if got.scope != "window" {
		t.Errorf("SetOption scope = %q, want %q", got.scope, "window")
	}
	if got.name != "automatic-rename" {
		t.Errorf("SetOption name = %q, want %q", got.name, "automatic-rename")
	}
	if got.value != "off" {
		t.Errorf("SetOption value = %q, want %q", got.value, "off")
	}
}

// TestSetWindowOption_Alias_Setw verifies that the setw alias works.
func TestSetWindowOption_Alias_Setw(t *testing.T) {
	b := newBackend()
	res := dispatch("setw", []string{"automatic-rename", "off"}, b)
	if res.Err != nil {
		t.Fatalf("setw returned error: %v", res.Err)
	}
	if len(b.setOptions) != 1 || b.setOptions[0].scope != "window" {
		t.Errorf("setw did not set window scope: %+v", b.setOptions)
	}
}

// TestSetWindowOption_Unset verifies the -u flag unsets the option.
func TestSetWindowOption_Unset(t *testing.T) {
	b := newBackend()
	res := dispatch("set-window-option", []string{"-u", "automatic-rename"}, b)
	if res.Err != nil {
		t.Fatalf("set-window-option -u returned error: %v", res.Err)
	}
	if len(b.unsetOptions) != 1 {
		t.Fatalf("expected 1 UnsetOption call, got %d", len(b.unsetOptions))
	}
	got := b.unsetOptions[0]
	if got[0] != "window" || got[1] != "automatic-rename" {
		t.Errorf("UnsetOption(%q, %q): unexpected args", got[0], got[1])
	}
}

// TestShowWindowOptions_ListsWindowScopedOptions verifies that
// show-window-options lists options at window scope.
func TestShowWindowOptions_ListsWindowScopedOptions(t *testing.T) {
	b := newBackend()
	b.optionEntries = []command.OptionEntry{
		{Name: "automatic-rename", Value: "on"},
		{Name: "monitor-activity", Value: "off"},
	}
	res := dispatch("show-window-options", nil, b)
	if res.Err != nil {
		t.Fatalf("show-window-options returned error: %v", res.Err)
	}
	if !strings.Contains(res.Output, "automatic-rename") {
		t.Errorf("show-window-options output %q missing 'automatic-rename'", res.Output)
	}
	if !strings.Contains(res.Output, "monitor-activity") {
		t.Errorf("show-window-options output %q missing 'monitor-activity'", res.Output)
	}
}

// TestShowWindowOptions_Alias_Showw verifies the showw alias works.
func TestShowWindowOptions_Alias_Showw(t *testing.T) {
	b := newBackend()
	b.optionEntries = []command.OptionEntry{
		{Name: "automatic-rename", Value: "on"},
	}
	res := dispatch("showw", nil, b)
	if res.Err != nil {
		t.Fatalf("showw returned error: %v", res.Err)
	}
	if !strings.Contains(res.Output, "automatic-rename") {
		t.Errorf("showw output %q missing 'automatic-rename'", res.Output)
	}
}

// TestShowWindowOptions_Filter verifies that a positional option name filters
// the output to just that option.
func TestShowWindowOptions_Filter(t *testing.T) {
	b := newBackend()
	b.optionEntries = []command.OptionEntry{
		{Name: "automatic-rename", Value: "on"},
		{Name: "monitor-activity", Value: "off"},
	}
	res := dispatch("show-window-options", []string{"automatic-rename"}, b)
	if res.Err != nil {
		t.Fatalf("show-window-options with filter returned error: %v", res.Err)
	}
	if !strings.Contains(res.Output, "automatic-rename") {
		t.Errorf("output %q missing 'automatic-rename'", res.Output)
	}
	if strings.Contains(res.Output, "monitor-activity") {
		t.Errorf("output %q should not contain 'monitor-activity'", res.Output)
	}
}

// TestShowWindowOptions_ValuesOnly verifies that -v shows only option values.
func TestShowWindowOptions_ValuesOnly(t *testing.T) {
	b := newBackend()
	b.optionEntries = []command.OptionEntry{
		{Name: "automatic-rename", Value: "on"},
	}
	res := dispatch("show-window-options", []string{"-v"}, b)
	if res.Err != nil {
		t.Fatalf("show-window-options -v returned error: %v", res.Err)
	}
	if strings.Contains(res.Output, "automatic-rename") {
		t.Errorf("show-window-options -v output %q should not contain option name", res.Output)
	}
	if !strings.Contains(res.Output, "on") {
		t.Errorf("show-window-options -v output %q missing value 'on'", res.Output)
	}
}

// TestShowHooks_ListsRegisteredHooks verifies that show-hooks lists hooks set
// with set-hook.
func TestShowHooks_ListsRegisteredHooks(t *testing.T) {
	b := newBackend()
	res := dispatch("set-hook", []string{"after-new-session", "new-session -s hooktest"}, b)
	if res.Err != nil {
		t.Fatalf("set-hook returned error: %v", res.Err)
	}
	res = dispatch("show-hooks", nil, b)
	if res.Err != nil {
		t.Fatalf("show-hooks returned error: %v", res.Err)
	}
	if !strings.Contains(res.Output, "after-new-session") {
		t.Errorf("show-hooks output %q missing 'after-new-session'", res.Output)
	}
	if !strings.Contains(res.Output, "new-session -s hooktest") {
		t.Errorf("show-hooks output %q missing hook command", res.Output)
	}
}

// TestShowHooks_EmptyWhenNoHooks verifies that show-hooks returns empty output
// when no hooks are registered.
func TestShowHooks_EmptyWhenNoHooks(t *testing.T) {
	b := newBackend()
	res := dispatch("show-hooks", nil, b)
	if res.Err != nil {
		t.Fatalf("show-hooks returned error: %v", res.Err)
	}
	if res.Output != "" {
		t.Errorf("show-hooks output = %q, want empty", res.Output)
	}
}
