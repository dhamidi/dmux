package render_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/render"
)

func TestBorderSetForName(t *testing.T) {
	tests := []struct {
		name     string
		wantVert rune
		wantHoriz rune
	}{
		{"single", '│', '─'},
		{"double", '║', '═'},
		{"heavy", '┃', '━'},
		{"simple", '|', '-'},
		{"padded", ' ', ' '},
		// Unknown names default to single.
		{"unknown", '│', '─'},
		{"", '│', '─'},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bs := render.BorderSetForName(tc.name)
			if bs.Vertical != tc.wantVert {
				t.Errorf("BorderSetForName(%q).Vertical = %q, want %q", tc.name, bs.Vertical, tc.wantVert)
			}
			if bs.Horizontal != tc.wantHoriz {
				t.Errorf("BorderSetForName(%q).Horizontal = %q, want %q", tc.name, bs.Horizontal, tc.wantHoriz)
			}
		})
	}
}

func TestBorderSetForName_AllFields(t *testing.T) {
	tests := []struct {
		name string
		want render.BorderSet
	}{
		{"single", render.BorderSetSingle},
		{"double", render.BorderSetDouble},
		{"heavy", render.BorderSetHeavy},
		{"simple", render.BorderSetSimple},
		{"padded", render.BorderSetPadded},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := render.BorderSetForName(tc.name)
			if got != tc.want {
				t.Errorf("BorderSetForName(%q) = %+v, want %+v", tc.name, got, tc.want)
			}
		})
	}
}
