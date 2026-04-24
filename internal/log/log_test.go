package log

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ensureClosed is a helper that closes the package logger and fails
// the test if Close returns an error. Deferred at the top of tests
// that call Open so one failing test never leaks global state into
// the next.
func ensureClosed(t *testing.T) {
	t.Helper()
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestOpenForCloseRecordsComponent(t *testing.T) {
	defer ensureClosed(t)
	path := filepath.Join(t.TempDir(), "dmux.log")
	if err := Open(Config{Path: path, Level: slog.LevelInfo}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	l := For("server")
	l.Info("hello", "answer", 42)
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	got := readFile(t, path)
	if !strings.Contains(got, "component=server") {
		t.Fatalf("missing component=server in %q", got)
	}
	if !strings.Contains(got, "answer=42") {
		t.Fatalf("missing answer=42 in %q", got)
	}
	if !strings.Contains(got, "msg=hello") {
		t.Fatalf("missing msg=hello in %q", got)
	}
}

func TestOpenTwiceReturnsErrAlreadyOpen(t *testing.T) {
	defer ensureClosed(t)
	path := filepath.Join(t.TempDir(), "dmux.log")
	if err := Open(Config{Path: path, Level: slog.LevelInfo}); err != nil {
		t.Fatalf("first Open: %v", err)
	}
	err := Open(Config{Path: path, Level: slog.LevelInfo})
	if !errors.Is(err, ErrAlreadyOpen) {
		t.Fatalf("got %v, want ErrAlreadyOpen", err)
	}
}

func TestCloseWithoutOpenIsIdempotent(t *testing.T) {
	if err := Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestOpenAfterCloseUsesNewPath(t *testing.T) {
	defer ensureClosed(t)
	dir := t.TempDir()
	first := filepath.Join(dir, "first.log")
	second := filepath.Join(dir, "second.log")

	if err := Open(Config{Path: first, Level: slog.LevelInfo}); err != nil {
		t.Fatalf("first Open: %v", err)
	}
	For("one").Info("first-record")
	if err := Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	if err := Open(Config{Path: second, Level: slog.LevelInfo}); err != nil {
		t.Fatalf("second Open: %v", err)
	}
	For("two").Info("second-record")
	if err := Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	a := readFile(t, first)
	b := readFile(t, second)
	if !strings.Contains(a, "first-record") {
		t.Fatalf("first file missing first-record: %q", a)
	}
	if strings.Contains(a, "second-record") {
		t.Fatalf("first file leaked second-record: %q", a)
	}
	if !strings.Contains(b, "second-record") {
		t.Fatalf("second file missing second-record: %q", b)
	}
	if strings.Contains(b, "first-record") {
		t.Fatalf("second file leaked first-record: %q", b)
	}
}

func TestLevelFiltersBelowThreshold(t *testing.T) {
	defer ensureClosed(t)
	path := filepath.Join(t.TempDir(), "dmux.log")
	if err := Open(Config{Path: path, Level: slog.LevelWarn}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	l := For("gate")
	l.Info("should-be-dropped")
	l.Warn("should-appear")
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	got := readFile(t, path)
	if strings.Contains(got, "should-be-dropped") {
		t.Fatalf("info record leaked below threshold: %q", got)
	}
	if !strings.Contains(got, "should-appear") {
		t.Fatalf("warn record missing: %q", got)
	}
}

func TestSlogDefaultIsInstalledAfterOpen(t *testing.T) {
	defer ensureClosed(t)
	path := filepath.Join(t.TempDir(), "dmux.log")
	if err := Open(Config{Path: path, Level: slog.LevelInfo}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	slog.Info("via-default", "k", "v")
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	got := readFile(t, path)
	if !strings.Contains(got, "via-default") {
		t.Fatalf("slog.Default did not route to log file: %q", got)
	}
	if !strings.Contains(got, "k=v") {
		t.Fatalf("missing k=v in %q", got)
	}
}

func TestSlogDefaultRestoredAfterClose(t *testing.T) {
	before := slog.Default()
	path := filepath.Join(t.TempDir(), "dmux.log")
	if err := Open(Config{Path: path, Level: slog.LevelInfo}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if slog.Default() == before {
		t.Fatalf("slog.Default unchanged after Open")
	}
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if slog.Default() != before {
		t.Fatalf("slog.Default not restored after Close")
	}
}

func TestOpenCreatesParentDirectory(t *testing.T) {
	defer ensureClosed(t)
	root := t.TempDir()
	path := filepath.Join(root, "a", "b", "c", "dmux.log")
	if err := Open(Config{Path: path, Level: slog.LevelInfo}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	For("setup").Info("created")
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("log file not created: %v", err)
	}
	got := readFile(t, path)
	if !strings.Contains(got, "created") {
		t.Fatalf("missing created record: %q", got)
	}
}

func TestForIsRaceFree(t *testing.T) {
	defer ensureClosed(t)
	path := filepath.Join(t.TempDir(), "dmux.log")
	if err := Open(Config{Path: path, Level: slog.LevelInfo}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	const workers = 16
	const perWorker = 50
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				l := For("worker", "id", id)
				l.Info("tick", "j", j)
			}
		}(i)
	}
	wg.Wait()
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestForAttachesExtraPairs(t *testing.T) {
	defer ensureClosed(t)
	path := filepath.Join(t.TempDir(), "dmux.log")
	if err := Open(Config{Path: path, Level: slog.LevelInfo}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	l := For("pane", "pane_id", 7, "session_id", "abc")
	l.Info("resize", "cols", 80, "rows", 24)
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	got := readFile(t, path)
	for _, want := range []string{
		"component=pane",
		"pane_id=7",
		"session_id=abc",
		"cols=80",
		"rows=24",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestForBeforeOpenDoesNotPanic(t *testing.T) {
	// Defensive: log.For is called by command-author code that may
	// run before Open during init/test setup. It must never be a
	// crash source.
	defer ensureClosed(t)
	l := For("early", "k", "v")
	if l == nil {
		t.Fatalf("For returned nil before Open")
	}
	// Emitting a record should not panic either, even though nothing
	// verifies where it lands.
	l.Info("ignored")
}
