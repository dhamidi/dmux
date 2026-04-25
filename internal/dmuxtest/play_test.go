package dmuxtest

import (
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
