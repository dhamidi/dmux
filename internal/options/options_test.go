package options_test

import (
	"errors"
	"testing"

	"github.com/dhamidi/dmux/internal/options"
)

func TestGetFallsBackToTableDefault(t *testing.T) {
	srv := options.NewServerOptions()
	sess := options.NewScopedOptions(options.SessionScope, srv)

	if got := sess.GetString("default-shell"); got != "/bin/sh" {
		t.Fatalf("default-shell: got %q, want %q", got, "/bin/sh")
	}
	if got := sess.GetString("default-terminal"); got != "xterm-256color" {
		t.Fatalf("default-terminal: got %q, want %q", got, "xterm-256color")
	}
	if got := sess.GetBool("status"); got != true {
		t.Fatalf("status default: got %v, want true", got)
	}
	if got := sess.GetString("status-position"); got != "bottom" {
		t.Fatalf("status-position default: got %q, want %q", got, "bottom")
	}
}

func TestGetWalksParentChain(t *testing.T) {
	srv := options.NewServerOptions()
	sess := options.NewScopedOptions(options.SessionScope, srv)

	if err := sess.Set("default-shell", options.NewString("/bin/zsh")); err != nil {
		t.Fatalf("Set on session: %v", err)
	}

	child := options.NewScopedOptions(options.SessionScope, sess)
	if got := child.GetString("default-shell"); got != "/bin/zsh" {
		t.Fatalf("child walk: got %q, want %q", got, "/bin/zsh")
	}

	// sess itself still reads its local value.
	if got := sess.GetString("default-shell"); got != "/bin/zsh" {
		t.Fatalf("sess local: got %q, want %q", got, "/bin/zsh")
	}
}

func TestSetScopeMismatch(t *testing.T) {
	srv := options.NewServerOptions()
	// status is SessionScope; writing it on server scope must reject.
	err := srv.Set("status", options.NewBool(false))
	if err == nil {
		t.Fatalf("Set on wrong scope returned nil, want error")
	}
	if !errors.Is(err, options.ErrScopeMismatch) {
		t.Fatalf("errors.Is(ErrScopeMismatch) failed: %v", err)
	}
	var oerr *options.Error
	if !errors.As(err, &oerr) {
		t.Fatalf("errors.As(*options.Error) failed: %v", err)
	}
	if oerr.Name != "status" {
		t.Fatalf("error name: got %q, want %q", oerr.Name, "status")
	}
	if oerr.Scope != options.ServerScope {
		t.Fatalf("error scope: got %v, want ServerScope", oerr.Scope)
	}
}

func TestSetInvalidChoice(t *testing.T) {
	srv := options.NewServerOptions()
	sess := options.NewScopedOptions(options.SessionScope, srv)
	err := sess.Set("status-position", options.NewChoice("middle"))
	if err == nil {
		t.Fatalf("Set with bad choice returned nil, want error")
	}
	if !errors.Is(err, options.ErrInvalidChoice) {
		t.Fatalf("errors.Is(ErrInvalidChoice) failed: %v", err)
	}
	var oerr *options.Error
	if !errors.As(err, &oerr) {
		t.Fatalf("errors.As(*options.Error) failed: %v", err)
	}
	if oerr.Choice != "middle" {
		t.Fatalf("error choice: got %q, want %q", oerr.Choice, "middle")
	}
}

func TestSetTypeMismatch(t *testing.T) {
	srv := options.NewServerOptions()
	sess := options.NewScopedOptions(options.SessionScope, srv)
	// status is Bool; a String value must be rejected.
	err := sess.Set("status", options.NewString("yes"))
	if err == nil {
		t.Fatalf("Set with bad type returned nil, want error")
	}
	if !errors.Is(err, options.ErrTypeMismatch) {
		t.Fatalf("errors.Is(ErrTypeMismatch) failed: %v", err)
	}
}

func TestSetUnknownOption(t *testing.T) {
	srv := options.NewServerOptions()
	sess := options.NewScopedOptions(options.SessionScope, srv)
	err := sess.Set("not-a-real-option", options.NewString("x"))
	if !errors.Is(err, options.ErrUnknownOption) {
		t.Fatalf("errors.Is(ErrUnknownOption) failed: %v", err)
	}
}

func TestGetStringPanicsOnWrongType(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("GetString on Bool option did not panic")
		}
	}()
	srv := options.NewServerOptions()
	sess := options.NewScopedOptions(options.SessionScope, srv)
	_ = sess.GetString("status")
}

func TestGetBoolPanicsOnWrongType(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("GetBool on String option did not panic")
		}
	}()
	srv := options.NewServerOptions()
	sess := options.NewScopedOptions(options.SessionScope, srv)
	_ = sess.GetBool("default-shell")
}

func TestUnsetRestoresParentValue(t *testing.T) {
	srv := options.NewServerOptions()
	sess := options.NewScopedOptions(options.SessionScope, srv)

	if err := sess.Set("default-shell", options.NewString("/bin/zsh")); err != nil {
		t.Fatalf("Set on session: %v", err)
	}

	child := options.NewScopedOptions(options.SessionScope, sess)
	if err := child.Set("default-shell", options.NewString("/bin/fish")); err != nil {
		t.Fatalf("Set on child: %v", err)
	}
	if got := child.GetString("default-shell"); got != "/bin/fish" {
		t.Fatalf("child local: got %q, want %q", got, "/bin/fish")
	}

	// Child unsets: Get should walk back to sess's value, not to the
	// Table default or to some stale leftover.
	if err := child.Unset("default-shell"); err != nil {
		t.Fatalf("Unset: %v", err)
	}
	if got := child.GetString("default-shell"); got != "/bin/zsh" {
		t.Fatalf("after unset: got %q, want %q (parent value)", got, "/bin/zsh")
	}
	// sess's value must be untouched.
	if got := sess.GetString("default-shell"); got != "/bin/zsh" {
		t.Fatalf("sess after child unset: got %q, want %q", got, "/bin/zsh")
	}
}

func TestUserOptionSetGet(t *testing.T) {
	srv := options.NewServerOptions()
	sess := options.NewScopedOptions(options.SessionScope, srv)

	if err := sess.Set("@client/shell", options.NewString("conn-17")); err != nil {
		t.Fatalf("Set user option: %v", err)
	}
	if got := sess.GetString("@client/shell"); got != "conn-17" {
		t.Fatalf("GetString user option: got %q, want %q", got, "conn-17")
	}
}

func TestUserOptionUnsetReadsEmpty(t *testing.T) {
	srv := options.NewServerOptions()
	if got := srv.GetString("@unset/key"); got != "" {
		t.Fatalf("unset user option: got %q, want empty string", got)
	}
}

func TestUserOptionAcceptedOnAnyScope(t *testing.T) {
	srv := options.NewServerOptions()
	if err := srv.Set("@hook/on-attach", options.NewString("flash-status")); err != nil {
		t.Fatalf("user option on ServerScope: %v", err)
	}
	sess := options.NewScopedOptions(options.SessionScope, srv)
	if got := sess.GetString("@hook/on-attach"); got != "flash-status" {
		t.Fatalf("inherit user option: got %q, want %q", got, "flash-status")
	}
}

func TestUserOptionRejectsNonString(t *testing.T) {
	srv := options.NewServerOptions()
	err := srv.Set("@bool", options.NewBool(true))
	if err == nil {
		t.Fatalf("non-string user option returned nil, want error")
	}
	if !errors.Is(err, options.ErrTypeMismatch) {
		t.Fatalf("errors.Is(ErrTypeMismatch) failed: %v", err)
	}
}

func TestUserOptionUnset(t *testing.T) {
	srv := options.NewServerOptions()
	sess := options.NewScopedOptions(options.SessionScope, srv)
	if err := sess.Set("@tmp", options.NewString("x")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !sess.IsSetLocally("@tmp") {
		t.Fatalf("IsSetLocally after Set returned false")
	}
	if err := sess.Unset("@tmp"); err != nil {
		t.Fatalf("Unset: %v", err)
	}
	if sess.IsSetLocally("@tmp") {
		t.Fatalf("IsSetLocally after Unset returned true")
	}
	if got := sess.GetString("@tmp"); got != "" {
		t.Fatalf("GetString after Unset: got %q, want empty", got)
	}
}

func TestIsSetLocally(t *testing.T) {
	srv := options.NewServerOptions()
	sess := options.NewScopedOptions(options.SessionScope, srv)
	if sess.IsSetLocally("default-shell") {
		t.Fatalf("fresh session reports default-shell as locally set")
	}
	if err := sess.Set("default-shell", options.NewString("/bin/zsh")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !sess.IsSetLocally("default-shell") {
		t.Fatalf("after Set, IsSetLocally is false")
	}
}
