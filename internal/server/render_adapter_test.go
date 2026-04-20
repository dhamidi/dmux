package server

import (
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/layout"
	"github.com/dhamidi/dmux/internal/pane"
	"github.com/dhamidi/dmux/internal/render"
	"github.com/dhamidi/dmux/internal/session"
)

// coloredPane implements session.Pane and returns a single cell with color data.
type coloredPane struct {
	id   session.PaneID
	grid pane.CellGrid
}

func (p *coloredPane) ID() session.PaneID              { return p.id }
func (p *coloredPane) Title() string                   { return "" }
func (p *coloredPane) Write(data []byte) error         { return nil }
func (p *coloredPane) SendKey(key keys.Key) error      { return nil }
func (p *coloredPane) SendMouse(ev keys.MouseEvent) error { return nil }
func (p *coloredPane) Resize(cols, rows int) error     { return nil }
func (p *coloredPane) Snapshot() pane.CellGrid         { return p.grid }
func (p *coloredPane) CaptureContent(_ bool) ([]byte, error) { return nil, nil }
func (p *coloredPane) Respawn(shell string) error      { return nil }
func (p *coloredPane) Close() error                    { return nil }
func (p *coloredPane) ShellPID() int                   { return 0 }
func (p *coloredPane) LastOutputAt() time.Time         { return time.Time{} }
func (p *coloredPane) ConsumeBell() bool               { return false }
func (p *coloredPane) ClearHistory()                   {}
func (p *coloredPane) ClearScreen() error              { return nil }

// TestRenderPaneAdapter_ColorFieldsPreserved verifies that color and attribute
// data set on pane.Cell values survives the renderPaneAdapter → render.CellGrid
// conversion without being discarded.
func TestRenderPaneAdapter_ColorFieldsPreserved(t *testing.T) {
	src := pane.Cell{
		Char:  'X',
		Fg:    pane.ColorRGB,
		Bg:    pane.ColorIndexed | 5,
		Attrs: render.AttrBold | render.AttrUnderline,
		FgR:   0x11,
		FgG:   0x22,
		FgB:   0x33,
		BgR:   0xaa,
		BgG:   0xbb,
		BgB:   0xcc,
	}

	cp := &coloredPane{
		id: session.PaneID(1),
		grid: pane.CellGrid{
			Rows:  1,
			Cols:  1,
			Cells: []pane.Cell{src},
		},
	}

	adapter := &renderPaneAdapter{
		p:    cp,
		rect: layout.Rect{X: 0, Y: 0, Width: 1, Height: 1},
	}

	got := adapter.Snapshot()
	if len(got.Cells) != 1 {
		t.Fatalf("Snapshot() returned %d cells, want 1", len(got.Cells))
	}
	c := got.Cells[0]
	if c.Char != src.Char {
		t.Errorf("Char = %q, want %q", c.Char, src.Char)
	}
	if c.Fg != src.Fg {
		t.Errorf("Fg = %v, want %v", c.Fg, src.Fg)
	}
	if c.Bg != src.Bg {
		t.Errorf("Bg = %v, want %v", c.Bg, src.Bg)
	}
	if c.Attrs != src.Attrs {
		t.Errorf("Attrs = %d, want %d", c.Attrs, src.Attrs)
	}
	if c.FgR != src.FgR || c.FgG != src.FgG || c.FgB != src.FgB {
		t.Errorf("FgRGB = (%02x,%02x,%02x), want (%02x,%02x,%02x)",
			c.FgR, c.FgG, c.FgB, src.FgR, src.FgG, src.FgB)
	}
	if c.BgR != src.BgR || c.BgG != src.BgG || c.BgB != src.BgB {
		t.Errorf("BgRGB = (%02x,%02x,%02x), want (%02x,%02x,%02x)",
			c.BgR, c.BgG, c.BgB, src.BgR, src.BgG, src.BgB)
	}
}
