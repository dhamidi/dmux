package style_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/render"
	"github.com/dhamidi/dmux/internal/style"
)

// --- attribute token tests ---

func TestParse_Bold(t *testing.T) {
	s := style.Parse("bold")
	if s.Attrs&style.AttrBold == 0 {
		t.Error("expected AttrBold to be set")
	}
}

func TestParse_Nobold(t *testing.T) {
	s := style.Parse("nobold")
	if s.AttrsClear&style.AttrBold == 0 {
		t.Error("expected AttrBold to be in AttrsClear")
	}
	if s.Attrs&style.AttrBold != 0 {
		t.Error("AttrBold should not be set when nobold")
	}
}

func TestParse_Italics(t *testing.T) {
	for _, token := range []string{"italics", "italic"} {
		s := style.Parse(token)
		if s.Attrs&style.AttrItalics == 0 {
			t.Errorf("%q: expected AttrItalics to be set", token)
		}
	}
}

func TestParse_Noitalics(t *testing.T) {
	s := style.Parse("noitalics")
	if s.AttrsClear&style.AttrItalics == 0 {
		t.Error("expected AttrItalics in AttrsClear")
	}
}

func TestParse_Underscore(t *testing.T) {
	for _, token := range []string{"underscore", "underline"} {
		s := style.Parse(token)
		if s.Attrs&style.AttrUnderscore == 0 {
			t.Errorf("%q: expected AttrUnderscore to be set", token)
		}
	}
}

func TestParse_Nounderscore(t *testing.T) {
	s := style.Parse("nounderscore")
	if s.AttrsClear&style.AttrUnderscore == 0 {
		t.Error("expected AttrUnderscore in AttrsClear")
	}
}

func TestParse_DoubleUnderscore(t *testing.T) {
	s := style.Parse("double-underscore")
	if s.Attrs&style.AttrDoubleUnderscore == 0 {
		t.Error("expected AttrDoubleUnderscore to be set")
	}
}

func TestParse_CurlyUnderscore(t *testing.T) {
	s := style.Parse("curly-underscore")
	if s.Attrs&style.AttrCurlyUnderscore == 0 {
		t.Error("expected AttrCurlyUnderscore to be set")
	}
}

func TestParse_Overline(t *testing.T) {
	s := style.Parse("overline")
	if s.Attrs&style.AttrOverline == 0 {
		t.Error("expected AttrOverline to be set")
	}
}

func TestParse_Nooverline(t *testing.T) {
	s := style.Parse("nooverline")
	if s.AttrsClear&style.AttrOverline == 0 {
		t.Error("expected AttrOverline in AttrsClear")
	}
}

func TestParse_Blink(t *testing.T) {
	s := style.Parse("blink")
	if s.Attrs&style.AttrBlink == 0 {
		t.Error("expected AttrBlink to be set")
	}
}

func TestParse_Noblink(t *testing.T) {
	s := style.Parse("noblink")
	if s.AttrsClear&style.AttrBlink == 0 {
		t.Error("expected AttrBlink in AttrsClear")
	}
}

func TestParse_Reverse(t *testing.T) {
	s := style.Parse("reverse")
	if s.Attrs&style.AttrReverse == 0 {
		t.Error("expected AttrReverse to be set")
	}
}

func TestParse_Noreverse(t *testing.T) {
	s := style.Parse("noreverse")
	if s.AttrsClear&style.AttrReverse == 0 {
		t.Error("expected AttrReverse in AttrsClear")
	}
}

func TestParse_Dim(t *testing.T) {
	s := style.Parse("dim")
	if s.Attrs&style.AttrDim == 0 {
		t.Error("expected AttrDim to be set")
	}
}

func TestParse_Nodim(t *testing.T) {
	s := style.Parse("nodim")
	if s.AttrsClear&style.AttrDim == 0 {
		t.Error("expected AttrDim in AttrsClear")
	}
}

func TestParse_Strikethrough(t *testing.T) {
	for _, token := range []string{"strikethrough", "strike", "struck"} {
		s := style.Parse(token)
		if s.Attrs&style.AttrStrikethrough == 0 {
			t.Errorf("%q: expected AttrStrikethrough to be set", token)
		}
	}
}

func TestParse_Nostrikethrough(t *testing.T) {
	s := style.Parse("nostrikethrough")
	if s.AttrsClear&style.AttrStrikethrough == 0 {
		t.Error("expected AttrStrikethrough in AttrsClear")
	}
}

func TestParse_Push(t *testing.T) {
	s := style.Parse("push")
	if !s.Push {
		t.Error("expected Push to be true")
	}
}

func TestParse_Pop(t *testing.T) {
	s := style.Parse("pop")
	if !s.Pop {
		t.Error("expected Pop to be true")
	}
}

func TestParse_None(t *testing.T) {
	s := style.Parse("none")
	if !s.None {
		t.Error("expected None to be true")
	}
}

func TestParse_Default(t *testing.T) {
	s := style.Parse("default")
	if !s.Default {
		t.Error("expected Default to be true")
	}
}

// --- compound marker tests ---

func TestParse_Compound(t *testing.T) {
	s := style.Parse("fg=red,bg=black,bold,italics")
	if !s.HasFg || s.Fg != render.ColorIndexed|1 {
		t.Errorf("fg: got HasFg=%v Fg=0x%04x, want HasFg=true Fg=0x%04x", s.HasFg, s.Fg, render.ColorIndexed|1)
	}
	if !s.HasBg || s.Bg != render.ColorIndexed|0 {
		t.Errorf("bg: got HasBg=%v Bg=0x%04x, want HasBg=true Bg=0x%04x", s.HasBg, s.Bg, render.ColorIndexed|0)
	}
	if s.Attrs&style.AttrBold == 0 {
		t.Error("expected AttrBold to be set")
	}
	if s.Attrs&style.AttrItalics == 0 {
		t.Error("expected AttrItalics to be set")
	}
}

func TestParse_NoneResetsAll(t *testing.T) {
	// none should clear previously accumulated attributes within the same marker.
	s := style.Parse("bold,italics,none")
	if s.None != true {
		t.Error("expected None=true after none token")
	}
	// After a lone none, attrs should be zero because none resets the accumulator.
	if s.Attrs != 0 {
		t.Errorf("expected Attrs=0 after none, got %d", s.Attrs)
	}
}

func TestParse_NoneFollowedByTokens(t *testing.T) {
	// Tokens after none are applied to the reset style.
	s := style.Parse("none,bold")
	if s.None {
		// After the whole marker is parsed, None should still be set to signal "reset first".
		// But bold should also be set.
	}
	if s.Attrs&style.AttrBold == 0 {
		t.Error("expected bold after none,bold")
	}
}

func TestParse_FgHexRGB(t *testing.T) {
	s := style.Parse("fg=#ff8000")
	if !s.HasFg {
		t.Fatal("expected HasFg=true")
	}
	if s.Fg != render.ColorRGB {
		t.Errorf("expected ColorRGB, got 0x%04x", s.Fg)
	}
	if s.FgR != 0xff || s.FgG != 0x80 || s.FgB != 0x00 {
		t.Errorf("got RGB (%d,%d,%d), want (255,128,0)", s.FgR, s.FgG, s.FgB)
	}
}

func TestParse_BgIndexed(t *testing.T) {
	s := style.Parse("bg=colour42")
	if !s.HasBg {
		t.Fatal("expected HasBg=true")
	}
	want := render.ColorIndexed | 42
	if s.Bg != want {
		t.Errorf("got 0x%04x, want 0x%04x", s.Bg, want)
	}
}

func TestParse_FgX11(t *testing.T) {
	s := style.Parse("fg=cornflowerblue")
	if !s.HasFg {
		t.Fatal("expected HasFg=true")
	}
	if s.Fg != render.ColorRGB {
		t.Errorf("expected ColorRGB, got 0x%04x", s.Fg)
	}
	if s.FgR != 100 || s.FgG != 149 || s.FgB != 237 {
		t.Errorf("got RGB (%d,%d,%d), want (100,149,237)", s.FgR, s.FgG, s.FgB)
	}
}

func TestParse_AlignCentre(t *testing.T) {
	for _, val := range []string{"align=centre", "align=center"} {
		s := style.Parse(val)
		if !s.HasAlign {
			t.Errorf("%q: expected HasAlign=true", val)
		}
		if s.Align != style.AlignCentre {
			t.Errorf("%q: expected AlignCentre", val)
		}
	}
}

func TestParse_AlignRight(t *testing.T) {
	s := style.Parse("align=right")
	if !s.HasAlign || s.Align != style.AlignRight {
		t.Error("expected AlignRight")
	}
}

// --- Apply / push-pop round-trip tests ---

func TestApply_SetsBold(t *testing.T) {
	base := style.Style{}
	delta := style.Parse("bold")
	result := style.Apply(base, delta)
	if result.Attrs&style.AttrBold == 0 {
		t.Error("expected AttrBold after Apply(bold)")
	}
}

func TestApply_ClearsBoldWithNobold(t *testing.T) {
	base := style.Apply(style.Style{}, style.Parse("bold"))
	result := style.Apply(base, style.Parse("nobold"))
	if result.Attrs&style.AttrBold != 0 {
		t.Error("expected AttrBold cleared after nobold")
	}
}

func TestApply_NoneResetsStyle(t *testing.T) {
	base := style.Apply(style.Style{}, style.Parse("bold,fg=red"))
	result := style.Apply(base, style.Parse("none"))
	if result.Attrs != 0 || result.HasFg {
		t.Error("expected style reset after none")
	}
}

func TestApply_DefaultResetsFgBg(t *testing.T) {
	base := style.Apply(style.Style{}, style.Parse("fg=red,bg=blue"))
	result := style.Apply(base, style.Parse("default"))
	if result.HasFg || result.Fg != render.ColorDefault {
		t.Error("expected Fg reset to default")
	}
	if result.HasBg || result.Bg != render.ColorDefault {
		t.Error("expected Bg reset to default")
	}
}

func TestApply_PreservesUnrelatedAttributes(t *testing.T) {
	base := style.Apply(style.Style{}, style.Parse("bold,fg=red"))
	// Applying only bg=blue should keep bold and fg.
	result := style.Apply(base, style.Parse("bg=blue"))
	if result.Attrs&style.AttrBold == 0 {
		t.Error("bold should be preserved")
	}
	if !result.HasFg || result.Fg != render.ColorIndexed|1 {
		t.Error("fg should be preserved")
	}
	if !result.HasBg {
		t.Error("bg should be set")
	}
}

func TestPushPop_RoundTrip(t *testing.T) {
	// Simulate the push/pop stack logic that callers implement.
	current := style.Apply(style.Style{}, style.Parse("bold,fg=red"))

	var stack []style.Style

	// Push
	pushDelta := style.Parse("push")
	if pushDelta.Push {
		stack = append(stack, current)
	}

	// Apply some changes.
	current = style.Apply(current, style.Parse("fg=blue,nobold"))

	if current.Attrs&style.AttrBold != 0 {
		t.Error("bold should be cleared after nobold")
	}
	if current.Fg != render.ColorIndexed|4 {
		t.Errorf("fg should be blue (0x%04x), got 0x%04x", render.ColorIndexed|4, current.Fg)
	}

	// Pop
	popDelta := style.Parse("pop")
	if popDelta.Pop && len(stack) > 0 {
		current = stack[len(stack)-1]
		stack = stack[:len(stack)-1]
	}

	// Current should be back to bold+fg=red.
	if current.Attrs&style.AttrBold == 0 {
		t.Error("bold should be restored after pop")
	}
	if current.Fg != render.ColorIndexed|1 {
		t.Errorf("fg should be red (0x%04x) after pop, got 0x%04x", render.ColorIndexed|1, current.Fg)
	}
}
