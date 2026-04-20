// Package style implements a tmux-compatible style string parser for #[...]
// markers embedded in format strings.
//
// A style marker has the form #[attr,attr,...] where each attr is one of:
//
//	fg=<colour>         foreground colour
//	bg=<colour>         background colour
//	bold / nobold
//	italics / italic / noitalics
//	underscore / underline / nounderscore
//	double-underscore
//	curly-underscore
//	overline / nooverline
//	blink / noblink
//	reverse / noreverse
//	dim / nodim
//	strikethrough / strike / struck / nostrikethrough
//	none                reset all attributes and colours
//	default             reset fg/bg to terminal default
//	push                push current style onto stack
//	pop                 pop style from stack
//	align=left|centre|right
//
// Multiple attributes may be combined with commas:
//
//	#[fg=red,bg=black,bold,italics]
//
// Parse parses the content inside the brackets (without the surrounding #[]).
package style

import "strings"

// SGR attribute bitmask constants for [Style.Attrs] and [Style.AttrsClear].
const (
	AttrBold            uint16 = 1 << 0
	AttrItalics         uint16 = 1 << 1
	AttrUnderscore      uint16 = 1 << 2
	AttrDoubleUnderscore uint16 = 1 << 3
	AttrCurlyUnderscore  uint16 = 1 << 4
	AttrOverline        uint16 = 1 << 5
	AttrBlink           uint16 = 1 << 6
	AttrReverse         uint16 = 1 << 7
	AttrDim             uint16 = 1 << 8
	AttrStrikethrough   uint16 = 1 << 9
)

// Align is the horizontal alignment directive carried by a style marker.
type Align uint8

const (
	AlignLeft   Align = iota // default
	AlignCentre              // align=centre or align=center
	AlignRight               // align=right
)

// Style is the parsed representation of a single #[...] style marker.
//
// Style acts as a delta to be applied to an ambient style:
//   - Attrs bits are OR-ed into the current attribute set.
//   - AttrsClear bits are AND-NOT-ed from the current attribute set.
//   - HasFg/HasBg indicate whether Fg/Bg were explicitly set by this marker.
//   - None resets the ambient style entirely (including attributes).
//   - Default resets Fg/Bg to the terminal default.
//   - Push/Pop signal stack operations; no other fields need to be set.
type Style struct {
	// Fg is the foreground colour. Meaningful when HasFg is true.
	Fg    Color
	// FgR, FgG, FgB are the RGB components when Fg == render.ColorRGB.
	FgR, FgG, FgB uint8

	// Bg is the background colour. Meaningful when HasBg is true.
	Bg    Color
	// BgR, BgG, BgB are the RGB components when Bg == render.ColorRGB.
	BgR, BgG, BgB uint8

	// Attrs contains the SGR attribute bits that this marker turns on.
	Attrs uint16
	// AttrsClear contains the SGR attribute bits that this marker turns off
	// (e.g. nobold sets the AttrBold bit here instead of in Attrs).
	AttrsClear uint16

	// HasFg is true when this marker explicitly sets a foreground colour.
	HasFg bool
	// HasBg is true when this marker explicitly sets a background colour.
	HasBg bool

	// Align is the horizontal alignment, if specified.
	Align Align
	// HasAlign is true when this marker explicitly sets alignment.
	HasAlign bool

	// Push and Pop are mutually exclusive stack-operation flags.
	Push bool
	Pop  bool

	// None, when true, instructs the caller to reset all style state.
	None bool
	// Default, when true, instructs the caller to reset Fg/Bg to terminal default.
	Default bool
}

// Parse parses the comma-separated attribute list that appears inside a
// #[...] marker.  s must be the content between the brackets, without the
// surrounding #[ and ].
//
// Unrecognised tokens and malformed colour values are silently ignored.
func Parse(s string) Style {
	var st Style
	for _, token := range strings.Split(s, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		applyToken(&st, token)
	}
	return st
}

// applyToken applies a single attribute token to st.
func applyToken(st *Style, token string) {
	// Handle key=value tokens first.
	if eq := strings.IndexByte(token, '='); eq >= 0 {
		key := token[:eq]
		val := token[eq+1:]
		switch key {
		case "fg":
			c, r, g, b, ok := ParseColour(val)
			if ok {
				st.Fg, st.FgR, st.FgG, st.FgB = c, r, g, b
				st.HasFg = true
			}
		case "bg":
			c, r, g, b, ok := ParseColour(val)
			if ok {
				st.Bg, st.BgR, st.BgG, st.BgB = c, r, g, b
				st.HasBg = true
			}
		case "align":
			switch strings.ToLower(val) {
			case "left":
				st.Align = AlignLeft
				st.HasAlign = true
			case "centre", "center":
				st.Align = AlignCentre
				st.HasAlign = true
			case "right":
				st.Align = AlignRight
				st.HasAlign = true
			}
		}
		return
	}

	// Plain keyword tokens.
	switch token {
	case "none":
		// Reset everything; subsequent tokens in the same marker still apply.
		*st = Style{None: true}
	case "default":
		st.Default = true
	case "push":
		st.Push = true
	case "pop":
		st.Pop = true

	case "bold":
		st.Attrs |= AttrBold
		st.AttrsClear &^= AttrBold
	case "nobold":
		st.AttrsClear |= AttrBold
		st.Attrs &^= AttrBold

	case "italics", "italic":
		st.Attrs |= AttrItalics
		st.AttrsClear &^= AttrItalics
	case "noitalics":
		st.AttrsClear |= AttrItalics
		st.Attrs &^= AttrItalics

	case "underscore", "underline":
		st.Attrs |= AttrUnderscore
		st.AttrsClear &^= AttrUnderscore
	case "nounderscore":
		st.AttrsClear |= AttrUnderscore
		st.Attrs &^= AttrUnderscore

	case "double-underscore":
		st.Attrs |= AttrDoubleUnderscore
		st.AttrsClear &^= AttrDoubleUnderscore

	case "curly-underscore":
		st.Attrs |= AttrCurlyUnderscore
		st.AttrsClear &^= AttrCurlyUnderscore

	case "overline":
		st.Attrs |= AttrOverline
		st.AttrsClear &^= AttrOverline
	case "nooverline":
		st.AttrsClear |= AttrOverline
		st.Attrs &^= AttrOverline

	case "blink":
		st.Attrs |= AttrBlink
		st.AttrsClear &^= AttrBlink
	case "noblink":
		st.AttrsClear |= AttrBlink
		st.Attrs &^= AttrBlink

	case "reverse":
		st.Attrs |= AttrReverse
		st.AttrsClear &^= AttrReverse
	case "noreverse":
		st.AttrsClear |= AttrReverse
		st.Attrs &^= AttrReverse

	case "dim":
		st.Attrs |= AttrDim
		st.AttrsClear &^= AttrDim
	case "nodim":
		st.AttrsClear |= AttrDim
		st.Attrs &^= AttrDim

	case "strikethrough", "strike", "struck":
		st.Attrs |= AttrStrikethrough
		st.AttrsClear &^= AttrStrikethrough
	case "nostrikethrough":
		st.AttrsClear |= AttrStrikethrough
		st.Attrs &^= AttrStrikethrough
	}
}

// Apply merges delta onto current, returning the resulting style.
// This helper is used by callers (e.g. status.renderLine) to maintain a
// running style as they walk a string containing multiple #[...] markers.
func Apply(current, delta Style) Style {
	if delta.None {
		return Style{}
	}
	result := current
	if delta.Default {
		result.Fg = 0 // render.ColorDefault
		result.HasFg = false
		result.Bg = 0 // render.ColorDefault
		result.HasBg = false
	}
	if delta.HasFg {
		result.Fg = delta.Fg
		result.FgR, result.FgG, result.FgB = delta.FgR, delta.FgG, delta.FgB
		result.HasFg = true
	}
	if delta.HasBg {
		result.Bg = delta.Bg
		result.BgR, result.BgG, result.BgB = delta.BgR, delta.BgG, delta.BgB
		result.HasBg = true
	}
	result.Attrs = (current.Attrs | delta.Attrs) &^ delta.AttrsClear
	if delta.HasAlign {
		result.Align = delta.Align
		result.HasAlign = true
	}
	// Push/Pop are not propagated into the running style.
	result.Push = false
	result.Pop = false
	result.None = false
	result.Default = false
	return result
}
