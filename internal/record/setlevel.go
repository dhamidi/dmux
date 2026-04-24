package record

// SetLevel updates the current recorder's verbosity. When no recorder
// is open, SetLevel is a no-op.
//
// Always available: scenario runners invoke it via the
// test-set-recorder-level command, and online inspector-style tools
// promote production recorders to Debug at runtime for targeted
// diagnosis. Debug-level call sites (EmitDebug) gate on
// CurrentLevel() so the cost is only paid while the level is raised.
func SetLevel(lv Level) {
	r := currentRecorder()
	if r == nil {
		return
	}
	r.level.Store(int32(lv))
}
