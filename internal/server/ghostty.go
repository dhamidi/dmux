package server

import (
	"io"

	libghostty "github.com/mitchellh/go-libghostty"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/render"
)

// ghosttyTerminal adapts *libghostty.Terminal to the pane.Terminal interface.
type ghosttyTerminal struct {
	term      *libghostty.Terminal
	rows      int
	cols      int
	ptyWriter io.Writer
}

// newGhosttyTerminal creates a new ghosttyTerminal with the given dimensions.
func newGhosttyTerminal(cols, rows int) (*ghosttyTerminal, error) {
	t, err := libghostty.NewTerminal(
		libghostty.WithSize(uint16(cols), uint16(rows)),
		libghostty.WithMaxScrollback(1000),
	)
	if err != nil {
		return nil, err
	}
	gt := &ghosttyTerminal{term: t, rows: rows, cols: cols}
	return gt, nil
}

// SetPTYWriter wires up the PTY write-back callback so that terminal
// query responses (e.g. DA, XTVERSION) are sent back to the shell.
func (g *ghosttyTerminal) SetPTYWriter(w io.Writer) {
	g.ptyWriter = w
	g.term.SetEffectWritePty(func(_ *libghostty.Terminal, data []byte) {
		if w != nil {
			_, _ = w.Write(data)
		}
	})
}

// Write implements pane.Terminal by feeding raw PTY output into the
// terminal parser.
func (g *ghosttyTerminal) Write(p []byte) (int, error) {
	return g.term.Write(p)
}

// Resize implements pane.Terminal.
func (g *ghosttyTerminal) Resize(cols, rows int) error {
	err := g.term.Resize(uint16(cols), uint16(rows), 0, 0)
	if err != nil {
		return err
	}
	g.cols = cols
	g.rows = rows
	return nil
}

// Title implements pane.Terminal.
func (g *ghosttyTerminal) Title() (string, error) {
	return g.term.Title()
}

// Snapshot implements pane.Terminal. It returns an immutable snapshot of
// the current visible viewport by iterating the active grid via GridRef.
func (g *ghosttyTerminal) Snapshot() pane.CellGrid {
	rows := g.rows
	cols := g.cols
	cells := make([]pane.Cell, rows*cols)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			ref, err := g.term.GridRef(libghostty.Point{
				Tag: libghostty.PointTagActive,
				X:   uint16(c),
				Y:   uint32(r),
			})
			if err != nil {
				continue
			}
			cell, err := ref.Cell()
			if err != nil {
				continue
			}
			cp, err := cell.Codepoint()
			if err != nil {
				continue
			}
			style, err := ref.Style()
			if err != nil {
				cells[r*cols+c] = pane.Cell{Char: rune(cp)}
				continue
			}
			cells[r*cols+c] = pane.Cell{
				Char:  rune(cp),
				Fg:    styleColorToPaneColor(style.FgColor()),
				Bg:    styleColorToPaneColor(style.BgColor()),
				Attrs: styleToAttrs(style),
				FgR:   styleColorRGB(style.FgColor(), 'R'),
				FgG:   styleColorRGB(style.FgColor(), 'G'),
				FgB:   styleColorRGB(style.FgColor(), 'B'),
				BgR:   styleColorRGB(style.BgColor(), 'R'),
				BgG:   styleColorRGB(style.BgColor(), 'G'),
				BgB:   styleColorRGB(style.BgColor(), 'B'),
			}
		}
	}
	return pane.CellGrid{Rows: rows, Cols: cols, Cells: cells}
}

// Close implements pane.Terminal.
func (g *ghosttyTerminal) Close() {
	g.term.Close()
}

// styleColorToPaneColor maps a libghostty.StyleColor to a pane.Color.
func styleColorToPaneColor(sc libghostty.StyleColor) pane.Color {
	switch sc.Tag {
	case libghostty.StyleColorPalette:
		return render.ColorIndexed | pane.Color(sc.Palette)
	case libghostty.StyleColorRGB:
		return render.ColorRGB
	default:
		return render.ColorDefault
	}
}

// styleColorRGB returns the R, G, or B byte of a StyleColor when it is RGB.
func styleColorRGB(sc libghostty.StyleColor, component rune) uint8 {
	if sc.Tag != libghostty.StyleColorRGB {
		return 0
	}
	switch component {
	case 'R':
		return sc.RGB.R
	case 'G':
		return sc.RGB.G
	case 'B':
		return sc.RGB.B
	}
	return 0
}

// styleToAttrs maps a libghostty.Style to the render.Attr* bitmask.
func styleToAttrs(s *libghostty.Style) uint16 {
	var attrs uint16
	if s.Bold() {
		attrs |= render.AttrBold
	}
	if s.Inverse() {
		attrs |= render.AttrReverse
	}
	if s.Blink() {
		attrs |= render.AttrBlink
	}
	if s.Faint() {
		attrs |= render.AttrDim
	}
	if s.Italic() {
		attrs |= render.AttrItalics
	}
	if s.Overline() {
		attrs |= render.AttrOverline
	}
	if s.Strikethrough() {
		attrs |= render.AttrStrikethrough
	}
	switch s.Underline() {
	case libghostty.UnderlineSingle, libghostty.UnderlineDotted, libghostty.UnderlineDashed:
		attrs |= render.AttrUnderline
	case libghostty.UnderlineDouble:
		attrs |= render.AttrDoubleUnderline
	case libghostty.UnderlineCurly:
		attrs |= render.AttrCurlyUnderline
	}
	return attrs
}

// ---------------------------------------------------------------------------
// ghosttyKeyEncoder
// ---------------------------------------------------------------------------

// ghosttyKeyEncoder adapts *libghostty.KeyEncoder to the pane.KeyEncoder
// interface.
type ghosttyKeyEncoder struct {
	enc *libghostty.KeyEncoder
}

// newGhosttyKeyEncoder creates a new ghosttyKeyEncoder.
func newGhosttyKeyEncoder() (*ghosttyKeyEncoder, error) {
	enc, err := libghostty.NewKeyEncoder()
	if err != nil {
		return nil, err
	}
	return &ghosttyKeyEncoder{enc: enc}, nil
}

// Encode implements pane.KeyEncoder.
func (g *ghosttyKeyEncoder) Encode(key keys.Key) ([]byte, error) {
	event, err := libghostty.NewKeyEvent()
	if err != nil {
		return nil, err
	}
	defer event.Close()

	event.SetAction(libghostty.KeyActionPress)
	event.SetKey(dmuxKeyCodeToLibghosttyKey(key.Code))
	event.SetMods(dmuxModsToLibghosttyMods(key.Mod))

	// For printable characters, provide the unmodified UTF-8 text.
	// Do not provide UTF-8 for control keys or special keys.
	if key.Code > 0 {
		ch := rune(key.Code)
		// Do not send C0 control characters as UTF-8 text.
		if ch >= 0x20 && ch != 0x7f {
			event.SetUTF8(string(ch))
		}
	}

	return g.enc.Encode(event)
}

// Close implements pane.KeyEncoder.
func (g *ghosttyKeyEncoder) Close() {
	g.enc.Close()
}

// dmuxKeyCodeToLibghosttyKey maps a keys.KeyCode to the corresponding
// libghostty.Key physical key constant.
func dmuxKeyCodeToLibghosttyKey(code keys.KeyCode) libghostty.Key {
	if code > 0 {
		// Unicode codepoint → physical key
		return unicodeToLibghosttyKey(rune(code))
	}
	switch code {
	case keys.CodeEnter:
		return libghostty.KeyEnter
	case keys.CodeEscape:
		return libghostty.KeyEscape
	case keys.CodeTab:
		return libghostty.KeyTab
	case keys.CodeBackspace:
		return libghostty.KeyBackspace
	case keys.CodeUp:
		return libghostty.KeyArrowUp
	case keys.CodeDown:
		return libghostty.KeyArrowDown
	case keys.CodeLeft:
		return libghostty.KeyArrowLeft
	case keys.CodeRight:
		return libghostty.KeyArrowRight
	case keys.CodeHome:
		return libghostty.KeyHome
	case keys.CodeEnd:
		return libghostty.KeyEnd
	case keys.CodePageUp:
		return libghostty.KeyPageUp
	case keys.CodePageDown:
		return libghostty.KeyPageDown
	case keys.CodeInsert:
		return libghostty.KeyInsert
	case keys.CodeDelete:
		return libghostty.KeyDelete
	case keys.CodeF1:
		return libghostty.KeyF1
	case keys.CodeF2:
		return libghostty.KeyF2
	case keys.CodeF3:
		return libghostty.KeyF3
	case keys.CodeF4:
		return libghostty.KeyF4
	case keys.CodeF5:
		return libghostty.KeyF5
	case keys.CodeF6:
		return libghostty.KeyF6
	case keys.CodeF7:
		return libghostty.KeyF7
	case keys.CodeF8:
		return libghostty.KeyF8
	case keys.CodeF9:
		return libghostty.KeyF9
	case keys.CodeF10:
		return libghostty.KeyF10
	case keys.CodeF11:
		return libghostty.KeyF11
	case keys.CodeF12:
		return libghostty.KeyF12
	case keys.CodeF13:
		return libghostty.KeyF13
	case keys.CodeF14:
		return libghostty.KeyF14
	case keys.CodeF15:
		return libghostty.KeyF15
	case keys.CodeF16:
		return libghostty.KeyF16
	case keys.CodeF17:
		return libghostty.KeyF17
	case keys.CodeF18:
		return libghostty.KeyF18
	case keys.CodeF19:
		return libghostty.KeyF19
	case keys.CodeF20:
		return libghostty.KeyF20
	case keys.CodeF21:
		return libghostty.KeyF21
	case keys.CodeF22:
		return libghostty.KeyF22
	case keys.CodeF23:
		return libghostty.KeyF23
	case keys.CodeF24:
		return libghostty.KeyF24
	}
	return libghostty.KeyUnidentified
}

// unicodeToLibghosttyKey maps a Unicode codepoint to the best-matching
// libghostty physical key constant.
func unicodeToLibghosttyKey(ch rune) libghostty.Key {
	switch {
	case ch >= 'a' && ch <= 'z':
		// KeyA through KeyZ are contiguous in the C enum.
		return libghostty.Key(int(libghostty.KeyA) + int(ch-'a'))
	case ch >= 'A' && ch <= 'Z':
		return libghostty.Key(int(libghostty.KeyA) + int(ch-'A'))
	case ch >= '0' && ch <= '9':
		return libghostty.Key(int(libghostty.KeyDigit0) + int(ch-'0'))
	case ch == ' ':
		return libghostty.KeySpace
	case ch == '`':
		return libghostty.KeyBackquote
	case ch == '\\':
		return libghostty.KeyBackslash
	case ch == '[':
		return libghostty.KeyBracketLeft
	case ch == ']':
		return libghostty.KeyBracketRight
	case ch == ',':
		return libghostty.KeyComma
	case ch == '=':
		return libghostty.KeyEqual
	case ch == '-':
		return libghostty.KeyMinus
	case ch == '.':
		return libghostty.KeyPeriod
	case ch == '\'':
		return libghostty.KeyQuote
	case ch == ';':
		return libghostty.KeySemicolon
	case ch == '/':
		return libghostty.KeySlash
	}
	return libghostty.KeyUnidentified
}

// dmuxModsToLibghosttyMods converts keys.Modifier bits to libghostty.Mods.
func dmuxModsToLibghosttyMods(m keys.Modifier) libghostty.Mods {
	var mods libghostty.Mods
	if m&keys.ModCtrl != 0 {
		mods |= libghostty.ModCtrl
	}
	if m&keys.ModAlt != 0 {
		mods |= libghostty.ModAlt
	}
	if m&keys.ModShift != 0 {
		mods |= libghostty.ModShift
	}
	return mods
}

// ---------------------------------------------------------------------------
// ghosttyMouseEncoder
// ---------------------------------------------------------------------------

// ghosttyMouseEncoder adapts *libghostty.MouseEncoder to the
// pane.MouseEncoder interface.
type ghosttyMouseEncoder struct {
	enc *libghostty.MouseEncoder
}

// newGhosttyMouseEncoder creates a new ghosttyMouseEncoder.
func newGhosttyMouseEncoder() (*ghosttyMouseEncoder, error) {
	enc, err := libghostty.NewMouseEncoder()
	if err != nil {
		return nil, err
	}
	// Use a reasonable default cell size so that pixel→cell conversion
	// in the encoder produces correct results.
	enc.SetOptSize(libghostty.MouseEncoderSize{
		CellWidth:  12,
		CellHeight: 24,
	})
	enc.SetOptTrackingMode(libghostty.MouseTrackingAny)
	enc.SetOptFormat(libghostty.MouseFormatSGR)
	return &ghosttyMouseEncoder{enc: enc}, nil
}

// Encode implements pane.MouseEncoder.
func (g *ghosttyMouseEncoder) Encode(ev keys.MouseEvent) ([]byte, error) {
	event, err := libghostty.NewMouseEvent()
	if err != nil {
		return nil, err
	}
	defer event.Close()

	event.SetAction(dmuxMouseActionToLibghostty(ev.Action))

	btn := dmuxMouseButtonToLibghostty(ev.Button)
	if btn == libghostty.MouseButtonUnknown {
		event.ClearButton()
	} else {
		event.SetButton(btn)
	}

	// Convert cell coordinates to surface-space pixels using the cell
	// dimensions configured in SetOptSize above.
	const cellW, cellH = 12, 24
	event.SetPosition(libghostty.MousePosition{
		X: float32(ev.Col*cellW) + cellW/2,
		Y: float32(ev.Row*cellH) + cellH/2,
	})

	return g.enc.Encode(event)
}

// Close implements pane.MouseEncoder.
func (g *ghosttyMouseEncoder) Close() {
	g.enc.Close()
}

// dmuxMouseActionToLibghostty converts keys.MouseAction to libghostty.MouseAction.
func dmuxMouseActionToLibghostty(a keys.MouseAction) libghostty.MouseAction {
	switch a {
	case keys.MousePress:
		return libghostty.MouseActionPress
	case keys.MouseRelease:
		return libghostty.MouseActionRelease
	case keys.MouseMotion:
		return libghostty.MouseActionMotion
	}
	return libghostty.MouseActionPress
}

// dmuxMouseButtonToLibghostty converts keys.MouseButton to
// libghostty.MouseButton.
func dmuxMouseButtonToLibghostty(b keys.MouseButton) libghostty.MouseButton {
	switch b {
	case keys.MouseLeft:
		return libghostty.MouseButtonLeft
	case keys.MouseMiddle:
		return libghostty.MouseButtonMiddle
	case keys.MouseRight:
		return libghostty.MouseButtonRight
	case keys.MouseWheelUp:
		// Wheel up/down are encoded as special buttons in SGR mouse protocol.
		// libghostty does not have explicit wheel-up/down constants; use
		// an unknown button and rely on the action field for motion events.
		return libghostty.MouseButtonUnknown
	case keys.MouseWheelDown:
		return libghostty.MouseButtonUnknown
	case keys.MouseButton4:
		return libghostty.MouseButtonFour
	case keys.MouseButton5:
		return libghostty.MouseButtonFive
	}
	return libghostty.MouseButtonUnknown
}
