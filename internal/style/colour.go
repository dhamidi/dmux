package style

import (
	"strconv"
	"strings"

	"github.com/dhamidi/dmux/internal/render"
)

// Color is a terminal colour value, identical to [render.Color].
// Using a type alias keeps style.Color and render.Color interchangeable
// so that Style.Fg / Style.Bg can be assigned directly to render.Cell fields.
type Color = render.Color

// ParseColour parses a colour string and returns its Color value together with
// optional RGB components (meaningful only when c == render.ColorRGB).
// ok is false when s is not a recognised colour.
//
// Recognised formats:
//
//   - Named basic:   black red green yellow blue magenta cyan white default
//   - Named bright:  brightblack brightred … brightwhite
//   - Indexed:       colour0 – colour255  (British spelling)
//   - Hex RGB:       #rrggbb
//   - X11 names:     cornflowerblue indianred … (full 147-name table)
func ParseColour(s string) (c Color, r, g, b uint8, ok bool) {
	s = strings.ToLower(strings.TrimSpace(s))

	// Named basic colours → ANSI 0–7
	switch s {
	case "default":
		return render.ColorDefault, 0, 0, 0, true
	case "black":
		return render.ColorIndexed | 0, 0, 0, 0, true
	case "red":
		return render.ColorIndexed | 1, 0, 0, 0, true
	case "green":
		return render.ColorIndexed | 2, 0, 0, 0, true
	case "yellow":
		return render.ColorIndexed | 3, 0, 0, 0, true
	case "blue":
		return render.ColorIndexed | 4, 0, 0, 0, true
	case "magenta":
		return render.ColorIndexed | 5, 0, 0, 0, true
	case "cyan":
		return render.ColorIndexed | 6, 0, 0, 0, true
	case "white":
		return render.ColorIndexed | 7, 0, 0, 0, true
	}

	// Named bright colours → ANSI 8–15
	switch s {
	case "brightblack":
		return render.ColorIndexed | 8, 0, 0, 0, true
	case "brightred":
		return render.ColorIndexed | 9, 0, 0, 0, true
	case "brightgreen":
		return render.ColorIndexed | 10, 0, 0, 0, true
	case "brightyellow":
		return render.ColorIndexed | 11, 0, 0, 0, true
	case "brightblue":
		return render.ColorIndexed | 12, 0, 0, 0, true
	case "brightmagenta":
		return render.ColorIndexed | 13, 0, 0, 0, true
	case "brightcyan":
		return render.ColorIndexed | 14, 0, 0, 0, true
	case "brightwhite":
		return render.ColorIndexed | 15, 0, 0, 0, true
	}

	// Indexed colour: colour0–colour255
	if strings.HasPrefix(s, "colour") {
		n, err := strconv.Atoi(s[6:])
		if err == nil && n >= 0 && n <= 255 {
			return render.ColorIndexed | Color(n), 0, 0, 0, true
		}
	}

	// Hex RGB: #rrggbb
	if len(s) == 7 && s[0] == '#' {
		rv, err1 := strconv.ParseUint(s[1:3], 16, 8)
		gv, err2 := strconv.ParseUint(s[3:5], 16, 8)
		bv, err3 := strconv.ParseUint(s[5:7], 16, 8)
		if err1 == nil && err2 == nil && err3 == nil {
			return render.ColorRGB, uint8(rv), uint8(gv), uint8(bv), true
		}
	}

	// X11 colour names
	if rgb, found := x11Colours[s]; found {
		return render.ColorRGB, rgb[0], rgb[1], rgb[2], true
	}

	return 0, 0, 0, 0, false
}

// x11Colours is the full 147-entry X11/CSS named-colour table.
// Keys are lower-case. Values are [R, G, B].
var x11Colours = map[string][3]uint8{
	"aliceblue":            {240, 248, 255},
	"antiquewhite":         {250, 235, 215},
	"aqua":                 {0, 255, 255},
	"aquamarine":           {127, 255, 212},
	"azure":                {240, 255, 255},
	"beige":                {245, 245, 220},
	"bisque":               {255, 228, 196},
	"blanchedalmond":       {255, 235, 205},
	"blueviolet":           {138, 43, 226},
	"brown":                {165, 42, 42},
	"burlywood":            {222, 184, 135},
	"cadetblue":            {95, 158, 160},
	"chartreuse":           {127, 255, 0},
	"chocolate":            {210, 105, 30},
	"coral":                {255, 127, 80},
	"cornflowerblue":       {100, 149, 237},
	"cornsilk":             {255, 248, 220},
	"crimson":              {220, 20, 60},
	"darkblue":             {0, 0, 139},
	"darkcyan":             {0, 139, 139},
	"darkgoldenrod":        {184, 134, 11},
	"darkgray":             {169, 169, 169},
	"darkgreen":            {0, 100, 0},
	"darkgrey":             {169, 169, 169},
	"darkkhaki":            {189, 183, 107},
	"darkmagenta":          {139, 0, 139},
	"darkolivegreen":       {85, 107, 47},
	"darkorange":           {255, 140, 0},
	"darkorchid":           {153, 50, 204},
	"darkred":              {139, 0, 0},
	"darksalmon":           {233, 150, 122},
	"darkseagreen":         {143, 188, 143},
	"darkslateblue":        {72, 61, 139},
	"darkslategray":        {47, 79, 79},
	"darkslategrey":        {47, 79, 79},
	"darkturquoise":        {0, 206, 209},
	"darkviolet":           {148, 0, 211},
	"deeppink":             {255, 20, 147},
	"deepskyblue":          {0, 191, 255},
	"dimgray":              {105, 105, 105},
	"dimgrey":              {105, 105, 105},
	"dodgerblue":           {30, 144, 255},
	"firebrick":            {178, 34, 34},
	"floralwhite":          {255, 250, 240},
	"forestgreen":          {34, 139, 34},
	"fuchsia":              {255, 0, 255},
	"gainsboro":            {220, 220, 220},
	"ghostwhite":           {248, 248, 255},
	"gold":                 {255, 215, 0},
	"goldenrod":            {218, 165, 32},
	"greenyellow":          {173, 255, 47},
	"honeydew":             {240, 255, 240},
	"hotpink":              {255, 105, 180},
	"indianred":            {205, 92, 92},
	"indigo":               {75, 0, 130},
	"ivory":                {255, 255, 240},
	"khaki":                {240, 230, 140},
	"lavender":             {230, 230, 250},
	"lavenderblush":        {255, 240, 245},
	"lawngreen":            {124, 252, 0},
	"lemonchiffon":         {255, 250, 205},
	"lightblue":            {173, 216, 230},
	"lightcoral":           {240, 128, 128},
	"lightcyan":            {224, 255, 255},
	"lightgoldenrodyellow": {250, 250, 210},
	"lightgray":            {211, 211, 211},
	"lightgreen":           {144, 238, 144},
	"lightgrey":            {211, 211, 211},
	"lightpink":            {255, 182, 193},
	"lightsalmon":          {255, 160, 122},
	"lightseagreen":        {32, 178, 170},
	"lightskyblue":         {135, 206, 250},
	"lightslategray":       {119, 136, 153},
	"lightslategrey":       {119, 136, 153},
	"lightsteelblue":       {176, 196, 222},
	"lightyellow":          {255, 255, 224},
	"lime":                 {0, 255, 0},
	"limegreen":            {50, 205, 50},
	"linen":                {250, 240, 230},
	"maroon":               {128, 0, 0},
	"mediumaquamarine":     {102, 205, 170},
	"mediumblue":           {0, 0, 205},
	"mediumorchid":         {186, 85, 211},
	"mediumpurple":         {147, 112, 219},
	"mediumseagreen":       {60, 179, 113},
	"mediumslateblue":      {123, 104, 238},
	"mediumspringgreen":    {0, 250, 154},
	"mediumturquoise":      {72, 209, 204},
	"mediumvioletred":      {199, 21, 133},
	"midnightblue":         {25, 25, 112},
	"mintcream":            {245, 255, 250},
	"mistyrose":            {255, 228, 225},
	"moccasin":             {255, 228, 181},
	"navajowhite":          {255, 222, 173},
	"navy":                 {0, 0, 128},
	"oldlace":              {253, 245, 230},
	"olive":                {128, 128, 0},
	"olivedrab":            {107, 142, 35},
	"orange":               {255, 165, 0},
	"orangered":            {255, 69, 0},
	"orchid":               {218, 112, 214},
	"palegoldenrod":        {238, 232, 170},
	"palegreen":            {152, 251, 152},
	"paleturquoise":        {175, 238, 238},
	"palevioletred":        {219, 112, 147},
	"papayawhip":           {255, 239, 213},
	"peachpuff":            {255, 218, 185},
	"peru":                 {205, 133, 63},
	"pink":                 {255, 192, 203},
	"plum":                 {221, 160, 221},
	"powderblue":           {176, 224, 230},
	"purple":               {128, 0, 128},
	"rosybrown":            {188, 143, 143},
	"royalblue":            {65, 105, 225},
	"saddlebrown":          {139, 69, 19},
	"salmon":               {250, 128, 114},
	"sandybrown":           {244, 164, 96},
	"seagreen":             {46, 139, 87},
	"seashell":             {255, 245, 238},
	"sienna":               {160, 82, 45},
	"silver":               {192, 192, 192},
	"skyblue":              {135, 206, 235},
	"slateblue":            {106, 90, 205},
	"slategray":            {112, 128, 144},
	"slategrey":            {112, 128, 144},
	"snow":                 {255, 250, 250},
	"springgreen":          {0, 255, 127},
	"steelblue":            {70, 130, 180},
	"tan":                  {210, 180, 140},
	"teal":                 {0, 128, 128},
	"thistle":              {216, 191, 216},
	"tomato":               {255, 99, 71},
	"turquoise":            {64, 224, 208},
	"violet":               {238, 130, 238},
	"wheat":                {245, 222, 179},
	"whitesmoke":           {245, 245, 245},
	"yellowgreen":          {154, 205, 50},
}
