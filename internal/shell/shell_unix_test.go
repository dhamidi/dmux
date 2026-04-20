//go:build !windows

package shell_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/shell"
)

func noEnv(string) (string, bool)  { return "", false }
func noFile(string) bool           { return false }
func hasFile(path string) func(string) bool {
	return func(p string) bool { return p == path }
}
func envWith(key, val string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		if k == key {
			return val, true
		}
		return "", false
	}
}

func TestDefault_UsesShellEnvVar(t *testing.T) {
	got := shell.Default(envWith("SHELL", "/usr/bin/zsh"), noFile)
	if got != "/usr/bin/zsh" {
		t.Errorf("expected /usr/bin/zsh, got %q", got)
	}
}

func TestDefault_FallsBackToBinSh(t *testing.T) {
	got := shell.Default(noEnv, hasFile("/bin/sh"))
	if got != "/bin/sh" {
		t.Errorf("expected /bin/sh, got %q", got)
	}
}

func TestDefault_FallsBackToShWhenNothingExists(t *testing.T) {
	got := shell.Default(noEnv, noFile)
	if got != "sh" {
		t.Errorf("expected sh, got %q", got)
	}
}

func TestDefault_EmptyShellEnvVarIsIgnored(t *testing.T) {
	got := shell.Default(envWith("SHELL", ""), hasFile("/bin/sh"))
	if got != "/bin/sh" {
		t.Errorf("expected /bin/sh when SHELL is empty, got %q", got)
	}
}

func TestDefault_PrefersShellEnvOverBinSh(t *testing.T) {
	got := shell.Default(envWith("SHELL", "/bin/bash"), hasFile("/bin/sh"))
	if got != "/bin/bash" {
		t.Errorf("expected /bin/bash, got %q", got)
	}
}
