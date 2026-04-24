//go:build dmuxtest

package record

// SetLevel updates the current recorder's verbosity. Scenario files
// invoke this through the test-set-recorder-level command. When no
// recorder is open, SetLevel is a no-op.
//
// SetLevel is only compiled under the dmuxtest build tag: production
// never promotes to Debug — Debug-level call sites are always no-ops
// in a release binary.
func SetLevel(lv Level) {
	r := currentRecorder()
	if r == nil {
		return
	}
	r.level.Store(int32(lv))
}
