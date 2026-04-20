package style_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/render"
	"github.com/dhamidi/dmux/internal/style"
)

func TestParseColour_Default(t *testing.T) {
	c, _, _, _, ok := style.ParseColour("default")
	if !ok {
		t.Fatal("expected ok")
	}
	if c != render.ColorDefault {
		t.Errorf("got %v, want ColorDefault", c)
	}
}

func TestParseColour_BasicNames(t *testing.T) {
	tests := []struct {
		name  string
		index uint16
	}{
		{"black", 0},
		{"red", 1},
		{"green", 2},
		{"yellow", 3},
		{"blue", 4},
		{"magenta", 5},
		{"cyan", 6},
		{"white", 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _, _, _, ok := style.ParseColour(tt.name)
			if !ok {
				t.Fatalf("expected ok for %q", tt.name)
			}
			want := render.ColorIndexed | render.Color(tt.index)
			if c != want {
				t.Errorf("got 0x%04x, want 0x%04x", c, want)
			}
		})
	}
}

func TestParseColour_BrightNames(t *testing.T) {
	tests := []struct {
		name  string
		index uint16
	}{
		{"brightblack", 8},
		{"brightred", 9},
		{"brightgreen", 10},
		{"brightyellow", 11},
		{"brightblue", 12},
		{"brightmagenta", 13},
		{"brightcyan", 14},
		{"brightwhite", 15},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _, _, _, ok := style.ParseColour(tt.name)
			if !ok {
				t.Fatalf("expected ok for %q", tt.name)
			}
			want := render.ColorIndexed | render.Color(tt.index)
			if c != want {
				t.Errorf("got 0x%04x, want 0x%04x", c, want)
			}
		})
	}
}

func TestParseColour_Indexed(t *testing.T) {
	tests := []struct {
		input string
		index uint16
	}{
		{"colour0", 0},
		{"colour7", 7},
		{"colour128", 128},
		{"colour255", 255},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			c, _, _, _, ok := style.ParseColour(tt.input)
			if !ok {
				t.Fatalf("expected ok for %q", tt.input)
			}
			want := render.ColorIndexed | render.Color(tt.index)
			if c != want {
				t.Errorf("got 0x%04x, want 0x%04x", c, want)
			}
		})
	}
}

func TestParseColour_IndexedOutOfRange(t *testing.T) {
	_, _, _, _, ok := style.ParseColour("colour256")
	if ok {
		t.Error("colour256 should not be ok")
	}
}

func TestParseColour_HexRGB(t *testing.T) {
	c, r, g, b, ok := style.ParseColour("#ff8000")
	if !ok {
		t.Fatal("expected ok")
	}
	if c != render.ColorRGB {
		t.Errorf("got colour 0x%04x, want ColorRGB", c)
	}
	if r != 0xff || g != 0x80 || b != 0x00 {
		t.Errorf("got RGB (%d,%d,%d), want (255,128,0)", r, g, b)
	}
}

func TestParseColour_HexRGBUpperCase(t *testing.T) {
	c, r, g, b, ok := style.ParseColour("#FF8000")
	if !ok {
		t.Fatal("expected ok")
	}
	if c != render.ColorRGB {
		t.Errorf("colour should be ColorRGB")
	}
	if r != 0xff || g != 0x80 || b != 0x00 {
		t.Errorf("got RGB (%d,%d,%d), want (255,128,0)", r, g, b)
	}
}

func TestParseColour_X11Name(t *testing.T) {
	tests := []struct {
		name    string
		r, g, b uint8
	}{
		{"cornflowerblue", 100, 149, 237},
		{"indianred", 205, 92, 92},
		{"tomato", 255, 99, 71},
		{"CornflowerBlue", 100, 149, 237}, // case-insensitive
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, r, g, b, ok := style.ParseColour(tt.name)
			if !ok {
				t.Fatalf("expected ok for %q", tt.name)
			}
			if c != render.ColorRGB {
				t.Errorf("expected ColorRGB, got 0x%04x", c)
			}
			if r != tt.r || g != tt.g || b != tt.b {
				t.Errorf("got (%d,%d,%d), want (%d,%d,%d)", r, g, b, tt.r, tt.g, tt.b)
			}
		})
	}
}

func TestParseColour_Unknown(t *testing.T) {
	_, _, _, _, ok := style.ParseColour("notacolour")
	if ok {
		t.Error("unknown colour should return ok=false")
	}
}

func TestParseColour_EmptyString(t *testing.T) {
	_, _, _, _, ok := style.ParseColour("")
	if ok {
		t.Error("empty string should return ok=false")
	}
}
