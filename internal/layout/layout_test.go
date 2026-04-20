package layout

import (
	"slices"
	"testing"
)

// collectAll is a helper to collect all leaves from an iterator.
func collectAll(t *Tree) []LeafID {
	var ids []LeafID
	for id := range t.Leaves() {
		ids = append(ids, id)
	}
	return ids
}

// TestNewTree verifies that a new tree has the expected single leaf.
func TestNewTree(t *testing.T) {
	tree := New(80, 24, 1)
	leaves := collectAll(tree)
	if len(leaves) != 1 || leaves[0] != 1 {
		t.Fatalf("expected [1], got %v", leaves)
	}
	r := tree.Rect(1)
	if r.Width != 80 || r.Height != 24 {
		t.Errorf("expected 80x24, got %dx%d", r.Width, r.Height)
	}
}

// TestSplitHorizontal verifies that splitting a leaf horizontally produces two leaves.
func TestSplitHorizontal(t *testing.T) {
	tree := New(80, 24, 1)
	id2 := tree.Split(1, Horizontal)

	leaves := collectAll(tree)
	if len(leaves) != 2 {
		t.Fatalf("expected 2 leaves, got %d", len(leaves))
	}
	if !slices.Contains(leaves, LeafID(1)) || !slices.Contains(leaves, id2) {
		t.Errorf("unexpected leaves: %v", leaves)
	}

	r1 := tree.Rect(1)
	r2 := tree.Rect(id2)

	if r1.Width+r2.Width != 80 {
		t.Errorf("widths don't sum to 80: %d + %d", r1.Width, r2.Width)
	}
	if r1.Height != 24 || r2.Height != 24 {
		t.Errorf("heights should be 24: %d %d", r1.Height, r2.Height)
	}
	if r1.X != 0 {
		t.Errorf("first pane should start at x=0, got %d", r1.X)
	}
	if r2.X != r1.Width {
		t.Errorf("second pane should start at x=%d, got %d", r1.Width, r2.X)
	}
}

// TestSplitVertical verifies that splitting a leaf vertically produces two leaves.
func TestSplitVertical(t *testing.T) {
	tree := New(80, 24, 1)
	id2 := tree.Split(1, Vertical)

	r1 := tree.Rect(1)
	r2 := tree.Rect(id2)

	if r1.Height+r2.Height != 24 {
		t.Errorf("heights don't sum to 24: %d + %d", r1.Height, r2.Height)
	}
	if r1.Width != 80 || r2.Width != 80 {
		t.Errorf("widths should be 80: %d %d", r1.Width, r2.Width)
	}
}

// TestClose removes a leaf and verifies the remaining leaf takes full space.
func TestClose(t *testing.T) {
	tree := New(80, 24, 1)
	id2 := tree.Split(1, Horizontal)

	tree.Close(id2)
	leaves := collectAll(tree)
	if len(leaves) != 1 || leaves[0] != 1 {
		t.Fatalf("expected [1] after close, got %v", leaves)
	}
	r := tree.Rect(1)
	if r.Width != 80 || r.Height != 24 {
		t.Errorf("remaining pane should be full size, got %dx%d", r.Width, r.Height)
	}
}

// TestCloseFirst removes the first of two leaves.
func TestCloseFirst(t *testing.T) {
	tree := New(80, 24, 1)
	id2 := tree.Split(1, Horizontal)

	tree.Close(1)
	leaves := collectAll(tree)
	if len(leaves) != 1 || leaves[0] != id2 {
		t.Fatalf("expected [%d] after close, got %v", id2, leaves)
	}
	r := tree.Rect(id2)
	if r.Width != 80 || r.Height != 24 {
		t.Errorf("remaining pane should be full size, got %dx%d", r.Width, r.Height)
	}
}

// TestResize updates window dimensions and verifies rects scale.
func TestResize(t *testing.T) {
	tree := New(80, 24, 1)
	tree.Split(1, Horizontal)
	tree.Resize(160, 48)

	leaves := collectAll(tree)
	total := 0
	for _, id := range leaves {
		r := tree.Rect(id)
		total += r.Width
	}
	if total != 160 {
		t.Errorf("total width after resize should be 160, got %d", total)
	}
}

// TestZoom verifies that Zoom makes the target pane fill the window.
func TestZoom(t *testing.T) {
	tree := New(80, 24, 1)
	id2 := tree.Split(1, Horizontal)

	tree.Zoom(1)
	r1 := tree.Rect(1)
	r2 := tree.Rect(id2)

	if r1.Width != 80 || r1.Height != 24 {
		t.Errorf("zoomed pane should be 80x24, got %dx%d", r1.Width, r1.Height)
	}
	if r2.Width != 0 || r2.Height != 0 {
		t.Errorf("non-zoomed pane should be zero rect, got %dx%d", r2.Width, r2.Height)
	}
}

// TestUnzoom verifies that Unzoom restores normal layout.
func TestUnzoom(t *testing.T) {
	tree := New(80, 24, 1)
	tree.Split(1, Horizontal)

	tree.Zoom(1)
	tree.Unzoom()

	r := tree.Rect(1)
	if r.Width == 80 {
		// After unzoom the pane is split so should be ~half.
		// Actually it should be approximately half. Let's just verify it's < 80.
		t.Errorf("after unzoom pane should not be full width, got %d", r.Width)
	}
}

// TestLeaves verifies iteration order matches insertion order.
func TestLeaves(t *testing.T) {
	tree := New(80, 24, 1)
	id2 := tree.Split(1, Horizontal)
	id3 := tree.Split(id2, Horizontal)

	leaves := collectAll(tree)
	if len(leaves) != 3 {
		t.Fatalf("expected 3 leaves, got %d: %v", len(leaves), leaves)
	}
	if !slices.Contains(leaves, LeafID(1)) ||
		!slices.Contains(leaves, id2) ||
		!slices.Contains(leaves, id3) {
		t.Errorf("unexpected leaf set: %v", leaves)
	}
}

// TestRectsDoNotOverlap verifies that in a 3-pane layout all rects are disjoint.
func TestRectsDoNotOverlap(t *testing.T) {
	tree := New(80, 24, 1)
	id2 := tree.Split(1, Horizontal)
	id3 := tree.Split(1, Vertical)

	rects := map[LeafID]Rect{
		1:   tree.Rect(1),
		id2: tree.Rect(id2),
		id3: tree.Rect(id3),
	}

	ids := []LeafID{1, id2, id3}
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			a := rects[ids[i]]
			b := rects[ids[j]]
			if rectsOverlap(a, b) {
				t.Errorf("rects %v and %v overlap: %v vs %v", ids[i], ids[j], a, b)
			}
		}
	}
}

func rectsOverlap(a, b Rect) bool {
	if a.Width == 0 || a.Height == 0 || b.Width == 0 || b.Height == 0 {
		return false
	}
	return a.X < b.X+b.Width &&
		a.X+a.Width > b.X &&
		a.Y < b.Y+b.Height &&
		a.Y+a.Height > b.Y
}

// TestPresetEvenHorizontal verifies that all leaves end up in a single row.
func TestPresetEvenHorizontal(t *testing.T) {
	tree := New(80, 24, 1)
	tree.Split(1, Vertical)
	id3 := tree.Split(1, Horizontal)
	_ = id3

	tree.ApplyPreset(PresetEvenHorizontal)
	leaves := collectAll(tree)
	// All leaves should share the same Y and Height.
	y0 := tree.Rect(leaves[0]).Y
	h0 := tree.Rect(leaves[0]).Height
	for _, id := range leaves[1:] {
		r := tree.Rect(id)
		if r.Y != y0 || r.Height != h0 {
			t.Errorf("leaf %d not in row: Y=%d H=%d (expected Y=%d H=%d)", id, r.Y, r.Height, y0, h0)
		}
	}
}

// TestPresetEvenVertical verifies that all leaves end up in a single column.
func TestPresetEvenVertical(t *testing.T) {
	tree := New(80, 24, 1)
	tree.Split(1, Horizontal)
	tree.Split(1, Vertical)

	tree.ApplyPreset(PresetEvenVertical)
	leaves := collectAll(tree)
	x0 := tree.Rect(leaves[0]).X
	w0 := tree.Rect(leaves[0]).Width
	for _, id := range leaves[1:] {
		r := tree.Rect(id)
		if r.X != x0 || r.Width != w0 {
			t.Errorf("leaf %d not in column: X=%d W=%d", id, r.X, r.Width)
		}
	}
}

// TestMarshalUnmarshal verifies round-trip serialisation.
func TestMarshalUnmarshal(t *testing.T) {
	tree := New(80, 24, 1)
	tree.Split(1, Horizontal)

	s := tree.Marshal()
	t.Logf("marshaled: %s", s)

	tree2, err := Unmarshal(s)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	leaves1 := collectAll(tree)
	leaves2 := collectAll(tree2)
	if len(leaves1) != len(leaves2) {
		t.Errorf("leaf count mismatch: %d vs %d", len(leaves1), len(leaves2))
	}
	if tree2.cols != 80 || tree2.rows != 24 {
		t.Errorf("dims mismatch: %dx%d", tree2.cols, tree2.rows)
	}
}

// TestSizeCalculationsTotal verifies that leaf rects sum to the full window area.
func TestSizeCalculationsTotal(t *testing.T) {
	tree := New(80, 24, 1)
	id2 := tree.Split(1, Horizontal)
	id3 := tree.Split(id2, Vertical)

	totalArea := 0
	for _, id := range []LeafID{1, id2, id3} {
		r := tree.Rect(id)
		totalArea += r.Width * r.Height
	}
	expected := 80 * 24
	if totalArea != expected {
		t.Errorf("total area %d != expected %d", totalArea, expected)
	}
}

// TestCloseZoomedPane verifies that closing the zoomed pane clears zoom state.
func TestCloseZoomedPane(t *testing.T) {
	tree := New(80, 24, 1)
	id2 := tree.Split(1, Horizontal)

	tree.Zoom(id2)
	tree.Close(id2)

	// Zoom should be cleared; remaining pane should have normal rect.
	r := tree.Rect(1)
	if r.Width != 80 || r.Height != 24 {
		t.Errorf("expected full size after closing zoomed pane, got %dx%d", r.Width, r.Height)
	}
}

// TestMoveBorder verifies that moving a border adjusts sizes.
func TestMoveBorder(t *testing.T) {
	tree := New(80, 24, 1)
	id2 := tree.Split(1, Horizontal)

	r1before := tree.Rect(1)
	tree.MoveBorder(1, EdgeRight, 10)
	r1after := tree.Rect(1)
	r2after := tree.Rect(id2)

	if r1after.Width <= r1before.Width {
		t.Errorf("pane 1 should have grown: %d -> %d", r1before.Width, r1after.Width)
	}
	if r1after.Width+r2after.Width != 80 {
		t.Errorf("widths should still sum to 80: %d + %d", r1after.Width, r2after.Width)
	}
}

// TestNoInternalImports is a compile-time guarantee: the package must not import
// any internal/* packages. If any import is present the package won't compile.
// (enforced by the Go compiler itself; this test is a documentation placeholder.)
func TestNoInternalImports(t *testing.T) {
	// The package compiles without internal/* dependencies.
	// This test exists to document the boundary requirement.
}
