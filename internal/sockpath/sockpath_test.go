package sockpath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// stagedEnv returns a Getenv func that reads only from the given map.
func stagedEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolveSocketPathVerbatim(t *testing.T) {
	// -S wins over everything else; not validated, not canonicalized.
	opts := Options{
		SocketPath: "/custom/path/to/socket",
		Label:      "ignored",
		Getenv:     stagedEnv(map[string]string{"DMUX": "/also/ignored"}),
	}
	got, err := Resolve(opts)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "/custom/path/to/socket" {
		t.Fatalf("got %q, want verbatim -S path", got)
	}
}

func TestResolveDMUXEnvNoComma(t *testing.T) {
	opts := Options{Getenv: stagedEnv(map[string]string{"DMUX": "/via/env"})}
	got, err := Resolve(opts)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "/via/env" {
		t.Fatalf("got %q, want /via/env", got)
	}
}

func TestResolveDMUXEnvWithComma(t *testing.T) {
	// tmux convention: only the path before the first comma is the
	// socket; whatever follows is a client-id suffix we ignore.
	opts := Options{Getenv: stagedEnv(map[string]string{"DMUX": "/via/env,17,0"})}
	got, err := Resolve(opts)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "/via/env" {
		t.Fatalf("got %q, want /via/env", got)
	}
}

func TestResolveInvalidLabel(t *testing.T) {
	cases := []string{
		"foo/bar",       // path separator
		"../escape",     // traversal attempt
		`back\slash`,    // Windows separator; rejected on both platforms
		".",             // would resolve to the subdir itself
		"..",            // would resolve to the parent dir
		"has\x00null",   // NUL is filename-illegal
	}
	for _, lbl := range cases {
		t.Run(lbl, func(t *testing.T) {
			_, err := Resolve(Options{Label: lbl, Getenv: stagedEnv(nil)})
			if !errors.Is(err, ErrInvalidLabel) {
				t.Fatalf("got %v, want ErrInvalidLabel", err)
			}
		})
	}
}

func TestResolveDMUXCommaPrefixFallsThrough(t *testing.T) {
	// $DMUX=",17,0" has no usable path before the comma. Rather
	// than return "" (not a valid socket address), Resolve falls
	// through to the default path.
	if runtime.GOOS == "windows" {
		t.Skip("Unix-layout test")
	}
	tmp := t.TempDir()
	opts := Options{Getenv: stagedEnv(map[string]string{
		"DMUX":   ",17,0",
		"TMPDIR": tmp,
	})}
	got, err := Resolve(opts)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Join(tmp, fmt.Sprintf("dmux-%d", os.Getuid()), "default")
	if got != want {
		t.Fatalf("got %q, want %q (should fall through to default)", got, want)
	}
}

func TestResolveDMUXEmptyFromGetenvFallsThrough(t *testing.T) {
	// Getenv returning "" for DMUX is the same as DMUX unset: take
	// the default branch.
	if runtime.GOOS == "windows" {
		t.Skip("Unix-layout test")
	}
	tmp := t.TempDir()
	opts := Options{Getenv: stagedEnv(map[string]string{
		"DMUX":   "",
		"TMPDIR": tmp,
	})}
	got, err := Resolve(opts)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.HasPrefix(got, tmp) {
		t.Fatalf("got %q, want prefix %q", got, tmp)
	}
}

func TestResolveSocketPathVerbatimWhitespace(t *testing.T) {
	// "verbatim" means no trimming. Lock in the contract.
	got, err := Resolve(Options{SocketPath: "  /leading/and/trailing  "})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "  /leading/and/trailing  " {
		t.Fatalf("got %q, want untrimmed path", got)
	}
}

func TestResolveDefaultLabel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-layout test")
	}
	tmp := t.TempDir()
	opts := Options{Getenv: stagedEnv(map[string]string{"TMPDIR": tmp})}
	got, err := Resolve(opts)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Join(tmp, fmt.Sprintf("dmux-%d", os.Getuid()), "default")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveCustomLabel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-layout test")
	}
	tmp := t.TempDir()
	opts := Options{Label: "work", Getenv: stagedEnv(map[string]string{"TMPDIR": tmp})}
	got, err := Resolve(opts)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.HasSuffix(got, "/work") {
		t.Fatalf("got %q, want path ending in /work", got)
	}
}

func TestResolveTmuxTmpdirWins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-layout test")
	}
	tmuxDir := t.TempDir()
	tmpDir := t.TempDir()
	opts := Options{Getenv: stagedEnv(map[string]string{
		"TMUX_TMPDIR": tmuxDir,
		"TMPDIR":      tmpDir,
	})}
	got, err := Resolve(opts)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.HasPrefix(got, tmuxDir) {
		t.Fatalf("got %q, want prefix %q (TMUX_TMPDIR should win)", got, tmuxDir)
	}
}

func TestResolveFallbackToSlashTmp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-layout test")
	}
	// No TMUX_TMPDIR, no TMPDIR -> /tmp. If /tmp/dmux-<uid>
	// already exists on the host with perms we can't control,
	// skip: we're testing the fallback branch selection, not the
	// stat check (which has its own tests).
	uidDir := fmt.Sprintf("/tmp/dmux-%d", os.Getuid())
	if _, err := os.Stat(uidDir); err == nil {
		t.Skipf("%s exists on host; fallback-branch test skipped to avoid host coupling", uidDir)
	}
	opts := Options{Getenv: stagedEnv(nil)}
	got, err := Resolve(opts)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.HasPrefix(got, "/tmp/") {
		t.Fatalf("got %q, want prefix /tmp/", got)
	}
}

func TestResolveTmpdirMissingIsOK(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-layout test")
	}
	// Parent dir of the uid-subdir exists, but the uid-subdir
	// itself does not. sockpath should succeed — the socket
	// package creates the subdir with correct perms later.
	tmp := t.TempDir()
	// Sanity: the uid-subdir must not exist inside our fresh TempDir.
	uidDir := filepath.Join(tmp, fmt.Sprintf("dmux-%d", os.Getuid()))
	if _, err := os.Stat(uidDir); !os.IsNotExist(err) {
		t.Fatalf("precondition: %s should not exist, stat err=%v", uidDir, err)
	}
	opts := Options{Getenv: stagedEnv(map[string]string{"TMPDIR": tmp})}
	if _, err := Resolve(opts); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
}

func TestResolveTmpdirWrongMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-layout test")
	}
	tmp := t.TempDir()
	uidDir := filepath.Join(tmp, fmt.Sprintf("dmux-%d", os.Getuid()))
	if err := os.Mkdir(uidDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	opts := Options{Getenv: stagedEnv(map[string]string{"TMPDIR": tmp})}
	_, err := Resolve(opts)
	if !errors.Is(err, ErrBadTmpdir) {
		t.Fatalf("got %v, want ErrBadTmpdir", err)
	}
}

func TestResolveTmpdirCorrectMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-layout test")
	}
	tmp := t.TempDir()
	uidDir := filepath.Join(tmp, fmt.Sprintf("dmux-%d", os.Getuid()))
	if err := os.Mkdir(uidDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	opts := Options{Label: "s", Getenv: stagedEnv(map[string]string{"TMPDIR": tmp})}
	got, err := Resolve(opts)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != filepath.Join(uidDir, "s") {
		t.Fatalf("got %q, want %q", got, filepath.Join(uidDir, "s"))
	}
}

func TestResolveTmpdirIsFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-layout test")
	}
	tmp := t.TempDir()
	uidDir := filepath.Join(tmp, fmt.Sprintf("dmux-%d", os.Getuid()))
	if err := os.WriteFile(uidDir, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	opts := Options{Getenv: stagedEnv(map[string]string{"TMPDIR": tmp})}
	_, err := Resolve(opts)
	if !errors.Is(err, ErrBadTmpdir) {
		t.Fatalf("got %v, want ErrBadTmpdir", err)
	}
}

func TestResolveNilGetenvUsesOSEnv(t *testing.T) {
	// With Getenv nil, Resolve falls back to os.Getenv. We exercise
	// this by setting DMUX for the process and expecting it to flow
	// through. Using Setenv's t.Cleanup semantics keeps the test
	// isolated.
	t.Setenv("DMUX", "/from/os/env")
	got, err := Resolve(Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "/from/os/env" {
		t.Fatalf("got %q, want /from/os/env", got)
	}
}
