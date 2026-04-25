package dmuxtest

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/dhamidi/dmux/internal/script"
)

// Play reads the scenario at path and runs each line against a
// fresh in-process server. Blank lines and lines whose first
// non-whitespace character is `#` are skipped. Every other line is
// tokenized and dispatched through script.RunLine; the first command
// to fail stops the scenario with t.Fatalf, reporting the source +
// 1-based line number and the underlying error.
//
// Play wires its own SpawnServer, so callers do not set up a
// Harness themselves. Callers that want to reuse a single server
// across multiple scenarios should construct a Harness directly and
// call script.Run with h.Dialer().
func Play(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("dmuxtest: read %s: %v", path, err)
	}
	PlayInline(t, path, string(data))
}

// PlayInline is Play's string-literal sibling: the scenario body is
// passed directly instead of read from disk, and name is used only
// for diagnostics (the path-shaped prefix in the failure message).
// Useful for small inline tests and for authoring tests of the
// scenario-oriented commands themselves.
func PlayInline(t *testing.T, name string, scriptBody string) {
	t.Helper()
	h := SpawnServer(t)
	if err := script.Run(context.Background(), h.Dialer(), strings.NewReader(scriptBody), script.RunOptions{Source: name}); err != nil {
		t.Fatalf("%v", err)
	}
}
