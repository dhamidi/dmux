package keys

// Action is the press/release/repeat state of a key event. The
// numeric values mirror libghostty-vt's GhosttyKeyAction so that an
// Action can be handed to the pane-side encoder unchanged.
type Action uint8

const (
	// ActionRelease corresponds to GHOSTTY_KEY_ACTION_RELEASE.
	ActionRelease Action = 0
	// ActionPress corresponds to GHOSTTY_KEY_ACTION_PRESS.
	ActionPress Action = 1
	// ActionRepeat corresponds to GHOSTTY_KEY_ACTION_REPEAT.
	ActionRepeat Action = 2
)

// String returns a short human-readable label for a. The wording is
// for logs and error messages; it is not a stable wire format.
func (a Action) String() string {
	switch a {
	case ActionRelease:
		return "release"
	case ActionPress:
		return "press"
	case ActionRepeat:
		return "repeat"
	default:
		return "unknown"
	}
}

// Mods is a bitmask of keyboard modifier state. Bit positions match
// the GHOSTTY_MODS_* constants so a Mods value can be passed through
// to the libghostty-vt encoder without translation.
//
// Side bits (Shift/Ctrl/Alt/Super) are only meaningful when the
// corresponding primary modifier bit is set; platforms that do not
// distinguish sides leave the side bit clear.
type Mods uint16

const (
	// ModShift is set when Shift is pressed.
	ModShift Mods = 1 << 0
	// ModCtrl is set when Ctrl is pressed.
	ModCtrl Mods = 1 << 1
	// ModAlt is set when Alt/Option is pressed.
	ModAlt Mods = 1 << 2
	// ModSuper is set when Super/Command/Windows is pressed.
	ModSuper Mods = 1 << 3
	// ModCapsLock is set when Caps Lock is active.
	ModCapsLock Mods = 1 << 4
	// ModNumLock is set when Num Lock is active.
	ModNumLock Mods = 1 << 5
	// ModShiftSide distinguishes right Shift from left when ModShift is set.
	ModShiftSide Mods = 1 << 6
	// ModCtrlSide distinguishes right Ctrl from left when ModCtrl is set.
	ModCtrlSide Mods = 1 << 7
	// ModAltSide distinguishes right Alt from left when ModAlt is set.
	ModAltSide Mods = 1 << 8
	// ModSuperSide distinguishes right Super from left when ModSuper is set.
	ModSuperSide Mods = 1 << 9
)

// Event is one semantic key input event from the termin parser,
// shaped to pass through libghostty-vt's key encoder without
// translation. The fields mirror GhosttyKeyEvent one-for-one.
type Event struct {
	// Action is press, release, or repeat.
	Action Action
	// Key is the layout-independent physical key.
	Key Key
	// Mods is the modifier bitmask at the time of the event.
	Mods Mods
	// Text is the UTF-8 text the keypress would produce under the
	// current layout. It is empty when the keypress produces no
	// character (function keys, modifiers, etc.).
	Text string
	// Unshifted is the layout-unmapped Unicode codepoint for the key,
	// or 0 if unknown. Used by Kitty keyboard protocol encoding.
	Unshifted rune
	// Consumed is the bitmask of modifiers already consumed by the
	// platform's keyboard layout to produce Text. The encoder strips
	// these from Mods before synthesizing modifier sequences.
	Consumed Mods
	// Composing reports whether the event is part of an ongoing IME
	// composition sequence. Composing events are delivered so the
	// encoder can suppress text output, but they never drive bindings.
	Composing bool
}

// Key is the layout-independent physical key code. Values match
// GhosttyKey one-for-one so a Key can be passed to libghostty-vt
// without translation. Names follow the W3C UI Events KeyboardEvent
// code standard (https://www.w3.org/TR/uievents-code).
type Key uint16

const (
	// KeyUnidentified is the zero value and means "no key" or "the
	// platform reported a key we do not recognize". Events carrying
	// this Key never drive bindings.
	KeyUnidentified Key = 0

	// Writing System Keys (W3C § 3.1.1).
	KeyBackquote     Key = 1
	KeyBackslash     Key = 2
	KeyBracketLeft   Key = 3
	KeyBracketRight  Key = 4
	KeyComma         Key = 5
	KeyDigit0        Key = 6
	KeyDigit1        Key = 7
	KeyDigit2        Key = 8
	KeyDigit3        Key = 9
	KeyDigit4        Key = 10
	KeyDigit5        Key = 11
	KeyDigit6        Key = 12
	KeyDigit7        Key = 13
	KeyDigit8        Key = 14
	KeyDigit9        Key = 15
	KeyEqual         Key = 16
	KeyIntlBackslash Key = 17
	KeyIntlRo        Key = 18
	KeyIntlYen       Key = 19
	KeyA             Key = 20
	KeyB             Key = 21
	KeyC             Key = 22
	KeyD             Key = 23
	KeyE             Key = 24
	KeyF             Key = 25
	KeyG             Key = 26
	KeyH             Key = 27
	KeyI             Key = 28
	KeyJ             Key = 29
	KeyK             Key = 30
	KeyL             Key = 31
	KeyM             Key = 32
	KeyN             Key = 33
	KeyO             Key = 34
	KeyP             Key = 35
	KeyQ             Key = 36
	KeyR             Key = 37
	KeyS             Key = 38
	KeyT             Key = 39
	KeyU             Key = 40
	KeyV             Key = 41
	KeyW             Key = 42
	KeyX             Key = 43
	KeyY             Key = 44
	KeyZ             Key = 45
	KeyMinus         Key = 46
	KeyPeriod        Key = 47
	KeyQuote         Key = 48
	KeySemicolon     Key = 49
	KeySlash         Key = 50

	// Functional Keys (W3C § 3.1.2).
	KeyAltLeft      Key = 51
	KeyAltRight     Key = 52
	KeyBackspace    Key = 53
	KeyCapsLock     Key = 54
	KeyContextMenu  Key = 55
	KeyControlLeft  Key = 56
	KeyControlRight Key = 57
	KeyEnter        Key = 58
	KeyMetaLeft     Key = 59
	KeyMetaRight    Key = 60
	KeyShiftLeft    Key = 61
	KeyShiftRight   Key = 62
	KeySpace        Key = 63
	KeyTab          Key = 64
	KeyConvert      Key = 65
	KeyKanaMode     Key = 66
	KeyNonConvert   Key = 67

	// Control Pad Section (W3C § 3.2).
	KeyDelete   Key = 68
	KeyEnd      Key = 69
	KeyHelp     Key = 70
	KeyHome     Key = 71
	KeyInsert   Key = 72
	KeyPageDown Key = 73
	KeyPageUp   Key = 74

	// Arrow Pad Section (W3C § 3.3).
	KeyArrowDown  Key = 75
	KeyArrowLeft  Key = 76
	KeyArrowRight Key = 77
	KeyArrowUp    Key = 78

	// Numpad Section (W3C § 3.4).
	KeyNumLock              Key = 79
	KeyNumpad0              Key = 80
	KeyNumpad1              Key = 81
	KeyNumpad2              Key = 82
	KeyNumpad3              Key = 83
	KeyNumpad4              Key = 84
	KeyNumpad5              Key = 85
	KeyNumpad6              Key = 86
	KeyNumpad7              Key = 87
	KeyNumpad8              Key = 88
	KeyNumpad9              Key = 89
	KeyNumpadAdd            Key = 90
	KeyNumpadBackspace      Key = 91
	KeyNumpadClear          Key = 92
	KeyNumpadClearEntry     Key = 93
	KeyNumpadComma          Key = 94
	KeyNumpadDecimal        Key = 95
	KeyNumpadDivide         Key = 96
	KeyNumpadEnter          Key = 97
	KeyNumpadEqual          Key = 98
	KeyNumpadMemoryAdd      Key = 99
	KeyNumpadMemoryClear    Key = 100
	KeyNumpadMemoryRecall   Key = 101
	KeyNumpadMemoryStore    Key = 102
	KeyNumpadMemorySubtract Key = 103
	KeyNumpadMultiply       Key = 104
	KeyNumpadParenLeft      Key = 105
	KeyNumpadParenRight     Key = 106
	KeyNumpadSubtract       Key = 107
	KeyNumpadSeparator      Key = 108
	KeyNumpadUp             Key = 109
	KeyNumpadDown           Key = 110
	KeyNumpadRight          Key = 111
	KeyNumpadLeft           Key = 112
	KeyNumpadBegin          Key = 113
	KeyNumpadHome           Key = 114
	KeyNumpadEnd            Key = 115
	KeyNumpadInsert         Key = 116
	KeyNumpadDelete         Key = 117
	KeyNumpadPageUp         Key = 118
	KeyNumpadPageDown       Key = 119

	// Function Section (W3C § 3.5).
	KeyEscape      Key = 120
	KeyF1          Key = 121
	KeyF2          Key = 122
	KeyF3          Key = 123
	KeyF4          Key = 124
	KeyF5          Key = 125
	KeyF6          Key = 126
	KeyF7          Key = 127
	KeyF8          Key = 128
	KeyF9          Key = 129
	KeyF10         Key = 130
	KeyF11         Key = 131
	KeyF12         Key = 132
	KeyF13         Key = 133
	KeyF14         Key = 134
	KeyF15         Key = 135
	KeyF16         Key = 136
	KeyF17         Key = 137
	KeyF18         Key = 138
	KeyF19         Key = 139
	KeyF20         Key = 140
	KeyF21         Key = 141
	KeyF22         Key = 142
	KeyF23         Key = 143
	KeyF24         Key = 144
	KeyF25         Key = 145
	KeyFn          Key = 146
	KeyFnLock      Key = 147
	KeyPrintScreen Key = 148
	KeyScrollLock  Key = 149
	KeyPause       Key = 150

	// Media Keys (W3C § 3.6).
	KeyBrowserBack        Key = 151
	KeyBrowserFavorites   Key = 152
	KeyBrowserForward     Key = 153
	KeyBrowserHome        Key = 154
	KeyBrowserRefresh     Key = 155
	KeyBrowserSearch      Key = 156
	KeyBrowserStop        Key = 157
	KeyEject              Key = 158
	KeyLaunchApp1         Key = 159
	KeyLaunchApp2         Key = 160
	KeyLaunchMail         Key = 161
	KeyMediaPlayPause     Key = 162
	KeyMediaSelect        Key = 163
	KeyMediaStop          Key = 164
	KeyMediaTrackNext     Key = 165
	KeyMediaTrackPrevious Key = 166
	KeyPower              Key = 167
	KeySleep              Key = 168
	KeyAudioVolumeDown    Key = 169
	KeyAudioVolumeMute    Key = 170
	KeyAudioVolumeUp      Key = 171
	KeyWakeUp             Key = 172

	// Legacy, Non-standard, and Special Keys (W3C § 3.7).
	KeyCopy  Key = 173
	KeyCut   Key = 174
	KeyPaste Key = 175
)

// keyNames maps Key values to short human-readable labels. Printable
// writing-system keys use the lowercased character the keycap shows;
// everything else uses the W3C UI Events code name without the
// section prefix. The strings are for logs and KeyCode.String — they
// are not a stable wire format and must not be parsed.
var keyNames = [...]string{
	KeyUnidentified: "Unidentified",

	KeyBackquote:     "`",
	KeyBackslash:     "\\",
	KeyBracketLeft:   "[",
	KeyBracketRight:  "]",
	KeyComma:         ",",
	KeyDigit0:        "0",
	KeyDigit1:        "1",
	KeyDigit2:        "2",
	KeyDigit3:        "3",
	KeyDigit4:        "4",
	KeyDigit5:        "5",
	KeyDigit6:        "6",
	KeyDigit7:        "7",
	KeyDigit8:        "8",
	KeyDigit9:        "9",
	KeyEqual:         "=",
	KeyIntlBackslash: "IntlBackslash",
	KeyIntlRo:        "IntlRo",
	KeyIntlYen:       "IntlYen",
	KeyA:             "a",
	KeyB:             "b",
	KeyC:             "c",
	KeyD:             "d",
	KeyE:             "e",
	KeyF:             "f",
	KeyG:             "g",
	KeyH:             "h",
	KeyI:             "i",
	KeyJ:             "j",
	KeyK:             "k",
	KeyL:             "l",
	KeyM:             "m",
	KeyN:             "n",
	KeyO:             "o",
	KeyP:             "p",
	KeyQ:             "q",
	KeyR:             "r",
	KeyS:             "s",
	KeyT:             "t",
	KeyU:             "u",
	KeyV:             "v",
	KeyW:             "w",
	KeyX:             "x",
	KeyY:             "y",
	KeyZ:             "z",
	KeyMinus:         "-",
	KeyPeriod:        ".",
	KeyQuote:         "'",
	KeySemicolon:     ";",
	KeySlash:         "/",

	KeyAltLeft:      "AltLeft",
	KeyAltRight:     "AltRight",
	KeyBackspace:    "Backspace",
	KeyCapsLock:     "CapsLock",
	KeyContextMenu:  "ContextMenu",
	KeyControlLeft:  "ControlLeft",
	KeyControlRight: "ControlRight",
	KeyEnter:        "Enter",
	KeyMetaLeft:     "MetaLeft",
	KeyMetaRight:    "MetaRight",
	KeyShiftLeft:    "ShiftLeft",
	KeyShiftRight:   "ShiftRight",
	KeySpace:        "Space",
	KeyTab:          "Tab",
	KeyConvert:      "Convert",
	KeyKanaMode:     "KanaMode",
	KeyNonConvert:   "NonConvert",

	KeyDelete:   "Delete",
	KeyEnd:      "End",
	KeyHelp:     "Help",
	KeyHome:     "Home",
	KeyInsert:   "Insert",
	KeyPageDown: "PageDown",
	KeyPageUp:   "PageUp",

	KeyArrowDown:  "ArrowDown",
	KeyArrowLeft:  "ArrowLeft",
	KeyArrowRight: "ArrowRight",
	KeyArrowUp:    "ArrowUp",

	KeyNumLock:              "NumLock",
	KeyNumpad0:              "Numpad0",
	KeyNumpad1:              "Numpad1",
	KeyNumpad2:              "Numpad2",
	KeyNumpad3:              "Numpad3",
	KeyNumpad4:              "Numpad4",
	KeyNumpad5:              "Numpad5",
	KeyNumpad6:              "Numpad6",
	KeyNumpad7:              "Numpad7",
	KeyNumpad8:              "Numpad8",
	KeyNumpad9:              "Numpad9",
	KeyNumpadAdd:            "NumpadAdd",
	KeyNumpadBackspace:      "NumpadBackspace",
	KeyNumpadClear:          "NumpadClear",
	KeyNumpadClearEntry:     "NumpadClearEntry",
	KeyNumpadComma:          "NumpadComma",
	KeyNumpadDecimal:        "NumpadDecimal",
	KeyNumpadDivide:         "NumpadDivide",
	KeyNumpadEnter:          "NumpadEnter",
	KeyNumpadEqual:          "NumpadEqual",
	KeyNumpadMemoryAdd:      "NumpadMemoryAdd",
	KeyNumpadMemoryClear:    "NumpadMemoryClear",
	KeyNumpadMemoryRecall:   "NumpadMemoryRecall",
	KeyNumpadMemoryStore:    "NumpadMemoryStore",
	KeyNumpadMemorySubtract: "NumpadMemorySubtract",
	KeyNumpadMultiply:       "NumpadMultiply",
	KeyNumpadParenLeft:      "NumpadParenLeft",
	KeyNumpadParenRight:     "NumpadParenRight",
	KeyNumpadSubtract:       "NumpadSubtract",
	KeyNumpadSeparator:      "NumpadSeparator",
	KeyNumpadUp:             "NumpadUp",
	KeyNumpadDown:           "NumpadDown",
	KeyNumpadRight:          "NumpadRight",
	KeyNumpadLeft:           "NumpadLeft",
	KeyNumpadBegin:          "NumpadBegin",
	KeyNumpadHome:           "NumpadHome",
	KeyNumpadEnd:            "NumpadEnd",
	KeyNumpadInsert:         "NumpadInsert",
	KeyNumpadDelete:         "NumpadDelete",
	KeyNumpadPageUp:         "NumpadPageUp",
	KeyNumpadPageDown:       "NumpadPageDown",

	KeyEscape:      "Escape",
	KeyF1:          "F1",
	KeyF2:          "F2",
	KeyF3:          "F3",
	KeyF4:          "F4",
	KeyF5:          "F5",
	KeyF6:          "F6",
	KeyF7:          "F7",
	KeyF8:          "F8",
	KeyF9:          "F9",
	KeyF10:         "F10",
	KeyF11:         "F11",
	KeyF12:         "F12",
	KeyF13:         "F13",
	KeyF14:         "F14",
	KeyF15:         "F15",
	KeyF16:         "F16",
	KeyF17:         "F17",
	KeyF18:         "F18",
	KeyF19:         "F19",
	KeyF20:         "F20",
	KeyF21:         "F21",
	KeyF22:         "F22",
	KeyF23:         "F23",
	KeyF24:         "F24",
	KeyF25:         "F25",
	KeyFn:          "Fn",
	KeyFnLock:      "FnLock",
	KeyPrintScreen: "PrintScreen",
	KeyScrollLock:  "ScrollLock",
	KeyPause:       "Pause",

	KeyBrowserBack:        "BrowserBack",
	KeyBrowserFavorites:   "BrowserFavorites",
	KeyBrowserForward:     "BrowserForward",
	KeyBrowserHome:        "BrowserHome",
	KeyBrowserRefresh:     "BrowserRefresh",
	KeyBrowserSearch:      "BrowserSearch",
	KeyBrowserStop:        "BrowserStop",
	KeyEject:              "Eject",
	KeyLaunchApp1:         "LaunchApp1",
	KeyLaunchApp2:         "LaunchApp2",
	KeyLaunchMail:         "LaunchMail",
	KeyMediaPlayPause:     "MediaPlayPause",
	KeyMediaSelect:        "MediaSelect",
	KeyMediaStop:          "MediaStop",
	KeyMediaTrackNext:     "MediaTrackNext",
	KeyMediaTrackPrevious: "MediaTrackPrevious",
	KeyPower:              "Power",
	KeySleep:              "Sleep",
	KeyAudioVolumeDown:    "AudioVolumeDown",
	KeyAudioVolumeMute:    "AudioVolumeMute",
	KeyAudioVolumeUp:      "AudioVolumeUp",
	KeyWakeUp:             "WakeUp",

	KeyCopy:  "Copy",
	KeyCut:   "Cut",
	KeyPaste: "Paste",
}

// String returns a short human-readable label for k. Printable
// writing-system keys come back as the lowercased keycap character
// ("a", "0", ";"); other keys use the W3C code name ("ArrowDown",
// "F1", "NumpadEnter"). Unknown values return "Unidentified". The
// output is stable enough for log readability but not intended as a
// round-trippable parse target.
func (k Key) String() string {
	if int(k) < len(keyNames) {
		if name := keyNames[k]; name != "" {
			return name
		}
	}
	return "Unidentified"
}
