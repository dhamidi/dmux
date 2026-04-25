package dmuxtest

import (
	"context"
	"os"
	"strings"
	"testing"

	// Blank imports populate the cmd registry for scenarios run in
	// this package. The set tracks what basic.scenario, its inline
	// twin, and echo.scenario actually use; kill-server is the
	// harness's own tear-down. Production binaries wire the full
	// command list in cmd/dmux/main.go, but tests only need what
	// they exercise.
	_ "github.com/dhamidi/dmux/internal/cmd/client"
	_ "github.com/dhamidi/dmux/internal/cmd/killserver"
	_ "github.com/dhamidi/dmux/internal/cmd/newsession"
	_ "github.com/dhamidi/dmux/internal/cmd/wait"
	"github.com/dhamidi/dmux/internal/script"
)

func TestBasicScenario(t *testing.T) {
	Play(t, "testdata/scenarios/basic.scenario")
}

func TestPlayInline(t *testing.T) {
	script := `
# Mirror of basic.scenario, constructed as a Go string literal.
new-session
client spawn c
client kill c
`
	PlayInline(t, "inline-basic", script)
}

func TestEchoScenario(t *testing.T) {
	Play(t, "testdata/scenarios/echo.scenario")
}

// TestEchoScenarioTwice proves the recorder lifecycle tolerates
// sequential server spawns in the same test binary: each server.Run
// owns its own Open/Close, and a second scenario in the same process
// does not trip ErrAlreadyOpen.
func TestEchoScenarioTwice(t *testing.T) {
	Play(t, "testdata/scenarios/echo.scenario")
	Play(t, "testdata/scenarios/echo.scenario")
}

// TestScriptModeViaStdinShape exercises the same code path
// cmd/dmux's stdin-script mode hits: an io.Reader fed straight into
// script.Run with a per-line dialer pointing at the harness server.
// Proves the production runner can drive new-session + wait
// pane.ready end-to-end without going through Play's wrapper.
func TestScriptModeViaStdinShape(t *testing.T) {
	h := SpawnServer(t)
	body := strings.NewReader("# inline\nnew-session\nwait pane.ready\n")
	if err := script.Run(context.Background(), h.Dialer(), body, script.RunOptions{Source: "<stdin>"}); err != nil {
		t.Fatalf("script.Run: %v", err)
	}
}

// TestScriptModeFromFile mirrors the file-path mode of
// cmd/dmux/main.go: open a real file and hand its reader to
// script.Run. The fixture lives alongside the other .scenario
// files so it can be invoked manually with the dmux binary too.
func TestScriptModeFromFile(t *testing.T) {
	h := SpawnServer(t)
	f, err := os.Open("testdata/scenarios/script-mode.scenario")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	if err := script.Run(context.Background(), h.Dialer(), f, script.RunOptions{Source: "testdata/scenarios/script-mode.scenario"}); err != nil {
		t.Fatalf("script.Run: %v", err)
	}
}
