package record

// SetLevel updates the current recorder's verbosity. When no recorder
// is open, SetLevel is a no-op.
//
// Always available: the `recorder set-level` command invokes it for
// scenario runners and interactive operators, and online
// inspector-style tools promote production recorders to Debug at
// runtime for targeted diagnosis. Debug-level call sites (EmitDebug)
// gate on CurrentLevel() so the cost is only paid while the level is
// raised.
func SetLevel(lv Level) {
	r := currentRecorder()
	if r == nil {
		return
	}
	r.level.Store(int32(lv))
}
