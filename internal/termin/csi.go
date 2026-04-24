package termin

import "github.com/dhamidi/dmux/internal/keys"

// csiFinalKey maps a CSI sequence whose final byte is one of the
// letters A..H to the corresponding keys.Key. These are the
// "cursor-key" sequences emitted by xterm and most legacy
// terminals in both the CSI (ESC [ A) and SS3 (ESC O A) forms.
var csiFinalKey = map[byte]keys.Key{
	'A': keys.KeyArrowUp,
	'B': keys.KeyArrowDown,
	'C': keys.KeyArrowRight,
	'D': keys.KeyArrowLeft,
	'E': keys.KeyNumpadBegin, // CSI E is "begin" (numpad 5 in no-numlock)
	'F': keys.KeyEnd,
	'H': keys.KeyHome,
	// P..S on SS3 / CSI become F1..F4.
	'P': keys.KeyF1,
	'Q': keys.KeyF2,
	'R': keys.KeyF3,
	'S': keys.KeyF4,
	// Focus in / out are handled separately by the caller.
}

// csiTildeKey maps the leading parameter of a "CSI n ~" sequence
// to a keys.Key. Entries come straight from the xterm control
// sequences documentation.
var csiTildeKey = map[int]keys.Key{
	1:  keys.KeyHome,
	2:  keys.KeyInsert,
	3:  keys.KeyDelete,
	4:  keys.KeyEnd,
	5:  keys.KeyPageUp,
	6:  keys.KeyPageDown,
	7:  keys.KeyHome,
	8:  keys.KeyEnd,
	11: keys.KeyF1,
	12: keys.KeyF2,
	13: keys.KeyF3,
	14: keys.KeyF4,
	15: keys.KeyF5,
	// xterm skips 16.
	17: keys.KeyF6,
	18: keys.KeyF7,
	19: keys.KeyF8,
	20: keys.KeyF9,
	21: keys.KeyF10,
	// xterm skips 22.
	23: keys.KeyF11,
	24: keys.KeyF12,
	25: keys.KeyF13,
	26: keys.KeyF14,
	// xterm skips 27.
	28: keys.KeyF15,
	29: keys.KeyF16,
	// xterm skips 30.
	31: keys.KeyF17,
	32: keys.KeyF18,
	33: keys.KeyF19,
	34: keys.KeyF20,
}

// xtermModsToMods decodes the 1-based xterm modifier parameter
// found in sequences like "CSI 1 ; n A". The xterm encoding is
// (Shift=1) + (Alt=2) + (Ctrl=4) + (Super=8), all off-by-one
// (so 2 is Shift-alone, 3 is Alt-alone, etc.). Values outside the
// 2..16 range are treated as no modifiers.
func xtermModsToMods(n int) keys.Mods {
	if n < 2 {
		return 0
	}
	n--
	var m keys.Mods
	if n&1 != 0 {
		m |= keys.ModShift
	}
	if n&2 != 0 {
		m |= keys.ModAlt
	}
	if n&4 != 0 {
		m |= keys.ModCtrl
	}
	if n&8 != 0 {
		m |= keys.ModSuper
	}
	return m
}

// kkpModsToMods decodes a KKP modifier parameter. KKP uses the
// same off-by-one base-1 encoding as xterm for the low nibble, so
// the implementation is the same. Reported as a separate helper
// because the call sites differ and future KKP extensions
// (CapsLock, NumLock, hyper/meta) may diverge.
func kkpModsToMods(n int) keys.Mods {
	return xtermModsToMods(n)
}

// kkpEventTypeToAction maps the KKP event-type sub-parameter
// (1=press, 2=repeat, 3=release) to keys.Action. 0 and other
// values default to press.
func kkpEventTypeToAction(n int) keys.Action {
	switch n {
	case 2:
		return keys.ActionRepeat
	case 3:
		return keys.ActionRelease
	default:
		return keys.ActionPress
	}
}

// kkpCodepointToKey maps a KKP functional-key codepoint (the
// first CSI-u parameter for non-text keys) to a keys.Key. KKP
// uses Unicode PUA codepoints for these; we cover the ones tmux
// actually binds. Unknown codepoints return KeyUnidentified.
func kkpCodepointToKey(cp rune) keys.Key {
	switch cp {
	case 57361:
		return keys.KeyPrintScreen
	case 57362:
		return keys.KeyScrollLock
	case 57363:
		return keys.KeyPause
	case 57364:
		return keys.KeyContextMenu
	case 57376:
		return keys.KeyF13
	case 57377:
		return keys.KeyF14
	case 57378:
		return keys.KeyF15
	case 57379:
		return keys.KeyF16
	case 57380:
		return keys.KeyF17
	case 57381:
		return keys.KeyF18
	case 57382:
		return keys.KeyF19
	case 57383:
		return keys.KeyF20
	case 57399, '0':
		return keys.KeyDigit0
	}
	return keys.KeyUnidentified
}
