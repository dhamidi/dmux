//go:build dmuxtest

package record_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/record"
)

func TestSetLevelPromotesDebug(t *testing.T) {
	if err := record.Open(record.Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = record.Close() })

	record.SetLevel(record.LevelDebug)
	if record.CurrentLevel() != record.LevelDebug {
		t.Fatalf("CurrentLevel=%v, want LevelDebug", record.CurrentLevel())
	}

	ch := record.Subscribe(context.Background(), nil)
	record.EmitDebug(context.Background(), "vt.feed", "pane", 0)

	got := collect(t, ch, 1, time.Second)
	if got[0].Name != "vt.feed" {
		t.Fatalf("EmitDebug did not deliver at LevelDebug: %v", names(got))
	}
}
