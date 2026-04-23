package termcaps

// Profile is the closed-set terminal capability classification. The
// u8 values are wire-stable: they travel in proto.Identify.Profile and
// proto.CapsUpdate.Profile. Do not reorder without a wire version
// bump.
type Profile uint8

const (
	Unknown         Profile = 0
	Ghostty         Profile = 1
	XTermJSModern   Profile = 2
	XTermJSLegacy   Profile = 3
	WindowsTerminal Profile = 4
)

func (p Profile) String() string {
	switch p {
	case Ghostty:
		return "ghostty"
	case XTermJSModern:
		return "xtermjs-modern"
	case XTermJSLegacy:
		return "xtermjs-legacy"
	case WindowsTerminal:
		return "windows-terminal"
	default:
		return "unknown"
	}
}

// Features is the flat boolean view consumed by termin, termout, and
// pane. Higher layers branch on Features — never on Profile directly —
// so that adding a new Profile only touches this file.
type Features struct {
	TrueColor       bool
	KKP             bool
	Sixel           bool
	KittyGraphics   bool
	OSC8            bool
	FocusEvents     bool
	BracketedPaste  bool
	ModifyOtherKeys bool
}

// Features derives the feature set for a profile. The matrix matches
// docs/m1.md; adding a profile means adding a branch here.
//
// M1 scope: callers only read TrueColor + OSC8 today. The rest are
// populated so that termin and termout can grow without widening this
// API later.
func (p Profile) Features() Features {
	switch p {
	case Ghostty:
		return Features{
			TrueColor:      true,
			KKP:            true,
			Sixel:          true,
			KittyGraphics:  true,
			OSC8:           true,
			FocusEvents:    true,
			BracketedPaste: true,
		}
	case XTermJSModern:
		return Features{
			TrueColor:      true,
			KKP:            true,
			Sixel:          true,
			OSC8:           true,
			FocusEvents:    true,
			BracketedPaste: true,
		}
	case XTermJSLegacy:
		return Features{
			TrueColor:       true,
			OSC8:            true,
			FocusEvents:     true,
			BracketedPaste:  true,
			ModifyOtherKeys: true,
		}
	case WindowsTerminal:
		return Features{
			TrueColor:       true,
			Sixel:           true,
			OSC8:            true,
			FocusEvents:     true,
			BracketedPaste:  true,
			ModifyOtherKeys: true,
		}
	default:
		return Features{
			TrueColor:       true,
			BracketedPaste:  true,
			ModifyOtherKeys: true,
		}
	}
}

// ProbeIO is the subset of io operations Detect needs: write a query
// byte sequence, read any reply with a short timeout. Kept narrow so
// tests and the eventual DA2/KKP prober can both satisfy it.
type ProbeIO interface {
	Write(p []byte) (int, error)
	ReadTimeout(p []byte) (int, error)
}

// Detect classifies the attached terminal. The real implementation
// follows doc.go's decision order (TERM_PROGRAM → WT_SESSION → DA2 →
// KKP → Unknown). For M1 this is a stub that returns Unknown so the
// walking skeleton has an end-to-end code path; the stub is safe
// because Unknown's Features set is the least-capable one, and every
// real terminal is a superset.
//
// TODO(m1:termcaps-detect): replace with the real probe. The server
// already reads Profile off Identify so the wire does not need to
// change when this lands.
func Detect(probe ProbeIO, env func(string) string) Profile {
	_ = probe
	_ = env
	return Unknown
}
