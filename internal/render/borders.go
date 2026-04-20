package render

// BorderSet holds the box-drawing characters used for one pane-border-lines style.
type BorderSet struct {
	Horizontal  rune // horizontal segment, e.g. ─
	Vertical    rune // vertical segment, e.g. │
	TopLeft     rune // top-left corner, e.g. ┌
	TopRight    rune // top-right corner, e.g. ┐
	BottomLeft  rune // bottom-left corner, e.g. └
	BottomRight rune // bottom-right corner, e.g. ┘
	TopT        rune // top T-junction (┬): horizontal with downward branch
	BottomT     rune // bottom T-junction (┴): horizontal with upward branch
	LeftT       rune // left T-junction (├): vertical with rightward branch
	RightT      rune // right T-junction (┤): vertical with leftward branch
	Cross       rune // cross-junction (┼): all four directions
}

// junctionChar returns the character from bs that matches the directions in
// which the border extends from the current cell.
func (bs BorderSet) junctionChar(hasTop, hasBottom, hasLeft, hasRight bool) rune {
	switch {
	case hasTop && hasBottom && hasLeft && hasRight:
		return bs.Cross
	case hasTop && hasBottom && !hasLeft && hasRight:
		return bs.LeftT
	case hasTop && hasBottom && hasLeft && !hasRight:
		return bs.RightT
	case !hasTop && hasBottom && hasLeft && hasRight:
		return bs.TopT
	case hasTop && !hasBottom && hasLeft && hasRight:
		return bs.BottomT
	case !hasTop && hasBottom && !hasLeft && hasRight:
		return bs.TopLeft
	case !hasTop && hasBottom && hasLeft && !hasRight:
		return bs.TopRight
	case hasTop && !hasBottom && !hasLeft && hasRight:
		return bs.BottomLeft
	case hasTop && !hasBottom && hasLeft && !hasRight:
		return bs.BottomRight
	case hasTop && hasBottom:
		return bs.Vertical
	case hasLeft && hasRight:
		return bs.Horizontal
	case hasTop || hasBottom:
		return bs.Vertical
	case hasLeft || hasRight:
		return bs.Horizontal
	default:
		return bs.Cross
	}
}

// Predefined border sets for the pane-border-lines option.
var (
	// BorderSetSingle uses thin single-line box-drawing characters.
	BorderSetSingle = BorderSet{
		Horizontal:  '─',
		Vertical:    '│',
		TopLeft:     '┌',
		TopRight:    '┐',
		BottomLeft:  '└',
		BottomRight: '┘',
		TopT:        '┬',
		BottomT:     '┴',
		LeftT:       '├',
		RightT:      '┤',
		Cross:       '┼',
	}

	// BorderSetDouble uses double-line box-drawing characters.
	BorderSetDouble = BorderSet{
		Horizontal:  '═',
		Vertical:    '║',
		TopLeft:     '╔',
		TopRight:    '╗',
		BottomLeft:  '╚',
		BottomRight: '╝',
		TopT:        '╦',
		BottomT:     '╩',
		LeftT:       '╠',
		RightT:      '╣',
		Cross:       '╬',
	}

	// BorderSetHeavy uses heavy (thick) box-drawing characters.
	BorderSetHeavy = BorderSet{
		Horizontal:  '━',
		Vertical:    '┃',
		TopLeft:     '┏',
		TopRight:    '┓',
		BottomLeft:  '┗',
		BottomRight: '┛',
		TopT:        '┳',
		BottomT:     '┻',
		LeftT:       '┣',
		RightT:      '┫',
		Cross:       '╋',
	}

	// BorderSetSimple uses ASCII-only characters.
	BorderSetSimple = BorderSet{
		Horizontal:  '-',
		Vertical:    '|',
		TopLeft:     '+',
		TopRight:    '+',
		BottomLeft:  '+',
		BottomRight: '+',
		TopT:        '+',
		BottomT:     '+',
		LeftT:       '+',
		RightT:      '+',
		Cross:       '+',
	}

	// BorderSetPadded uses space characters, creating invisible (padded) borders.
	BorderSetPadded = BorderSet{
		Horizontal:  ' ',
		Vertical:    ' ',
		TopLeft:     ' ',
		TopRight:    ' ',
		BottomLeft:  ' ',
		BottomRight: ' ',
		TopT:        ' ',
		BottomT:     ' ',
		LeftT:       ' ',
		RightT:      ' ',
		Cross:       ' ',
	}
)

// BorderSetForName returns the BorderSet corresponding to the given
// pane-border-lines option name. Unknown names default to BorderSetSingle.
func BorderSetForName(name string) BorderSet {
	switch name {
	case "single":
		return BorderSetSingle
	case "double":
		return BorderSetDouble
	case "heavy":
		return BorderSetHeavy
	case "simple":
		return BorderSetSimple
	case "padded":
		return BorderSetPadded
	default:
		return BorderSetSingle
	}
}
