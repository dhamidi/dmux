package record_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/dhamidi/dmux/internal/record"
)

func TestEmitBeforeOpenIsNoop(t *testing.T) {
	record.Emit(context.Background(), "pane.ready", "pane", 1)
	if got := record.Dropped(); got != 0 {
		t.Fatalf("Dropped()=%d, want 0 when no recorder is open", got)
	}
	if got := record.CurrentLevel(); got != record.LevelNormal {
		t.Fatalf("CurrentLevel()=%v, want LevelNormal when no recorder is open", got)
	}
}

func TestOpenCloseRoundTrip(t *testing.T) {
	if err := record.Open(record.Config{Logger: discardLogger()}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = record.Close() })

	if err := record.Open(record.Config{Logger: discardLogger()}); !errors.Is(err, record.ErrAlreadyOpen) {
		t.Fatalf("second Open returned %v, want ErrAlreadyOpen", err)
	}

	if err := record.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := record.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	if err := record.Open(record.Config{Logger: discardLogger()}); err != nil {
		t.Fatalf("Open after Close: %v", err)
	}
}

func TestEmitLogsToSlog(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	if err := record.Open(record.Config{Logger: logger}); err != nil {
		t.Fatalf("Open: %v", err)
	}

	record.Emit(context.Background(), "pane.ready", "pane", 42, "window", 7)

	if err := record.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "msg=pane.ready") {
		t.Fatalf("log missing event name, got: %q", out)
	}
	if !strings.Contains(out, "pane=42") {
		t.Fatalf("log missing pane field, got: %q", out)
	}
	if !strings.Contains(out, "window=7") {
		t.Fatalf("log missing window field, got: %q", out)
	}
}

func TestEmitDebugGatedByLevel(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	if err := record.Open(record.Config{Logger: logger}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = record.Close() })

	record.EmitDebug(context.Background(), "vt.feed", "pane", 0, "bytes", 8)

	if err := record.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if strings.Contains(buf.String(), "vt.feed") {
		t.Fatalf("EmitDebug produced a log record at LevelNormal: %q", buf.String())
	}
}

func TestBadKVKeysAreMarked(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	if err := record.Open(record.Config{Logger: logger}); err != nil {
		t.Fatalf("Open: %v", err)
	}

	record.Emit(context.Background(), "cmd.exec", "name", "attach-session", 999, "trailing")

	if err := record.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "!BADKEY#2=999") {
		t.Fatalf("non-string key not marked: %q", out)
	}
	if !strings.Contains(out, "!MISSING=trailing") {
		t.Fatalf("trailing unmatched key not marked: %q", out)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
