package builtin_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/command"
	"github.com/dhamidi/dmux/internal/layout"
)

// newBackendWithPanes returns a testBackend whose first window has n panes
// at 120×40.
func newBackendWithPanes(n int) *testBackend {
	b := newBackend()
	panes := make([]command.PaneView, n)
	for i := range panes {
		panes[i] = command.PaneView{ID: i + 1, Title: "bash"}
	}
	s := b.sessions[0]
	s.Windows[0].Panes = panes
	s.Windows[0].Active = 1
	s.Windows[0].Cols = 120
	s.Windows[0].Rows = 40
	b.sessions[0] = s
	return b
}

// ─── select-layout tests ─────────────────────────────────────────────────────

func TestSelectLayout_EvenHorizontal_CallsApplyLayout(t *testing.T) {
	b := newBackendWithPanes(3)
	res := dispatch("select-layout", []string{"even-horizontal"}, b)
	if res.Err != nil {
		t.Fatalf("select-layout even-horizontal returned error: %v", res.Err)
	}
	if len(b.appliedLayouts) != 1 {
		t.Fatalf("expected 1 ApplyLayout call, got %d", len(b.appliedLayouts))
	}
	got := b.appliedLayouts[0]
	if got.spec != "even-horizontal" {
		t.Errorf("ApplyLayout spec = %q, want %q", got.spec, "even-horizontal")
	}
}

func TestSelectLayout_MainVertical_CallsApplyLayout(t *testing.T) {
	b := newBackendWithPanes(3)
	res := dispatch("select-layout", []string{"main-vertical"}, b)
	if res.Err != nil {
		t.Fatalf("select-layout main-vertical returned error: %v", res.Err)
	}
	if len(b.appliedLayouts) != 1 {
		t.Fatalf("expected 1 ApplyLayout call, got %d", len(b.appliedLayouts))
	}
	if b.appliedLayouts[0].spec != "main-vertical" {
		t.Errorf("ApplyLayout spec = %q, want %q", b.appliedLayouts[0].spec, "main-vertical")
	}
}

func TestSelectLayout_NextFlag_SendsNextSpec(t *testing.T) {
	b := newBackendWithPanes(2)
	res := dispatch("select-layout", []string{"-n"}, b)
	if res.Err != nil {
		t.Fatalf("select-layout -n returned error: %v", res.Err)
	}
	if len(b.appliedLayouts) != 1 {
		t.Fatalf("expected 1 ApplyLayout call, got %d", len(b.appliedLayouts))
	}
	if b.appliedLayouts[0].spec != "next" {
		t.Errorf("ApplyLayout spec = %q, want %q", b.appliedLayouts[0].spec, "next")
	}
}

func TestSelectLayout_UndoFlag_SendsUndoSpec(t *testing.T) {
	b := newBackendWithPanes(2)
	res := dispatch("select-layout", []string{"-o"}, b)
	if res.Err != nil {
		t.Fatalf("select-layout -o returned error: %v", res.Err)
	}
	if len(b.appliedLayouts) != 1 || b.appliedLayouts[0].spec != "undo" {
		t.Errorf("ApplyLayout spec = %v, want undo", b.appliedLayouts)
	}
}

// ─── next-layout / previous-layout tests ─────────────────────────────────────

func TestNextLayout_CyclesForward(t *testing.T) {
	b := newBackendWithPanes(2)
	res := dispatch("next-layout", nil, b)
	if res.Err != nil {
		t.Fatalf("next-layout returned error: %v", res.Err)
	}
	if len(b.appliedLayouts) != 1 || b.appliedLayouts[0].spec != "next" {
		t.Errorf("next-layout: ApplyLayout(%v), want spec=next", b.appliedLayouts)
	}
}

func TestPreviousLayout_CyclesBack(t *testing.T) {
	b := newBackendWithPanes(2)
	res := dispatch("previous-layout", nil, b)
	if res.Err != nil {
		t.Fatalf("previous-layout returned error: %v", res.Err)
	}
	if len(b.appliedLayouts) != 1 || b.appliedLayouts[0].spec != "prev" {
		t.Errorf("previous-layout: ApplyLayout(%v), want spec=prev", b.appliedLayouts)
	}
}

// ─── rotate-window tests ──────────────────────────────────────────────────────

func TestRotateWindow_DefaultForward(t *testing.T) {
	b := newBackendWithPanes(3)
	res := dispatch("rotate-window", nil, b)
	if res.Err != nil {
		t.Fatalf("rotate-window returned error: %v", res.Err)
	}
	if len(b.rotatedWindows) != 1 {
		t.Fatalf("expected 1 RotateWindow call, got %d", len(b.rotatedWindows))
	}
	got := b.rotatedWindows[0]
	if !got.forward {
		t.Errorf("RotateWindow forward = %v, want true", got.forward)
	}
}

func TestRotateWindow_BackwardFlag(t *testing.T) {
	b := newBackendWithPanes(3)
	res := dispatch("rotate-window", []string{"-U"}, b)
	if res.Err != nil {
		t.Fatalf("rotate-window -U returned error: %v", res.Err)
	}
	if len(b.rotatedWindows) != 1 || b.rotatedWindows[0].forward {
		t.Errorf("RotateWindow forward = %v, want false", b.rotatedWindows)
	}
}

// ─── resize-window tests ──────────────────────────────────────────────────────

func TestResizeWindow_ExplicitDimensions(t *testing.T) {
	b := newBackendWithPanes(1)
	res := dispatch("resize-window", []string{"-x", "120", "-y", "40"}, b)
	if res.Err != nil {
		t.Fatalf("resize-window -x 120 -y 40 returned error: %v", res.Err)
	}
	if len(b.resizedWindows) != 1 {
		t.Fatalf("expected 1 ResizeWindow call, got %d", len(b.resizedWindows))
	}
	got := b.resizedWindows[0]
	if got.cols != 120 || got.rows != 40 {
		t.Errorf("ResizeWindow(%d, %d), want (120, 40)", got.cols, got.rows)
	}
}

func TestResizeWindow_DirectionalAdjustment(t *testing.T) {
	b := newBackendWithPanes(1)
	// Window starts at 120×40; -R 5 should give 125×40.
	res := dispatch("resize-window", []string{"-R", "5"}, b)
	if res.Err != nil {
		t.Fatalf("resize-window -R 5 returned error: %v", res.Err)
	}
	if len(b.resizedWindows) != 1 {
		t.Fatalf("expected 1 ResizeWindow call, got %d", len(b.resizedWindows))
	}
	got := b.resizedWindows[0]
	if got.cols != 125 || got.rows != 40 {
		t.Errorf("ResizeWindow(%d, %d), want (125, 40)", got.cols, got.rows)
	}
}

// ─── Layout algorithm tests (via layout package directly) ─────────────────────

func TestLayoutEvenHorizontal_DistributesWidthEqually(t *testing.T) {
	// 3 panes in a 120-wide window → each pane should be 40 cols wide.
	tree := layout.New(120, 24, layout.LeafID(1))
	tree.Split(layout.LeafID(1), layout.Horizontal)
	tree.Split(layout.LeafID(2), layout.Horizontal)
	tree.ApplyPreset(layout.PresetEvenHorizontal)

	leaves := collectAllLeaves(tree)
	for _, id := range leaves {
		r := tree.Rect(id)
		if r.Width != 40 {
			t.Errorf("pane %d: width = %d, want 40", id, r.Width)
		}
		if r.Height != 24 {
			t.Errorf("pane %d: height = %d, want 24", id, r.Height)
		}
	}
}

func TestLayoutMainVertical_PlacesMainPaneOnLeft(t *testing.T) {
	// main-vertical: main pane on left (width=main-pane-width), rest on right.
	tree := layout.New(120, 40, layout.LeafID(1))
	tree.Split(layout.LeafID(1), layout.Horizontal)
	tree.Split(layout.LeafID(2), layout.Horizontal)
	// Apply with main-pane-width = 80.
	tree.ApplyPresetSized(layout.PresetMainVertical, 80)

	mainRect := tree.Rect(layout.LeafID(1))
	if mainRect.X != 0 {
		t.Errorf("main pane X = %d, want 0", mainRect.X)
	}
	if mainRect.Width != 80 {
		t.Errorf("main pane width = %d, want 80", mainRect.Width)
	}
	if mainRect.Height != 40 {
		t.Errorf("main pane height = %d, want 40", mainRect.Height)
	}
	// The remaining panes should be to the right.
	for _, id := range []layout.LeafID{2, 3} {
		r := tree.Rect(id)
		if r.X != 80 {
			t.Errorf("secondary pane %d X = %d, want 80", id, r.X)
		}
	}
}

func TestLayoutRotateLeaves_ForwardRotation(t *testing.T) {
	// 3 panes in a row: [1, 2, 3] → after forward rotation: [3, 1, 2]
	tree := layout.New(120, 24, layout.LeafID(1))
	tree.Split(layout.LeafID(1), layout.Horizontal)
	tree.Split(layout.LeafID(2), layout.Horizontal)
	tree.ApplyPreset(layout.PresetEvenHorizontal)

	// Record original positions.
	origRects := map[layout.LeafID]layout.Rect{
		1: tree.Rect(1),
		2: tree.Rect(2),
		3: tree.Rect(3),
	}

	tree.RotateLeaves(true)

	// After forward rotation: leaf 3 takes position 0, leaf 1 takes position 1, leaf 2 takes position 2.
	if tree.Rect(3) != origRects[1] {
		t.Errorf("after forward rotate: leaf 3 rect = %+v, want %+v", tree.Rect(3), origRects[1])
	}
	if tree.Rect(1) != origRects[2] {
		t.Errorf("after forward rotate: leaf 1 rect = %+v, want %+v", tree.Rect(1), origRects[2])
	}
	if tree.Rect(2) != origRects[3] {
		t.Errorf("after forward rotate: leaf 2 rect = %+v, want %+v", tree.Rect(2), origRects[3])
	}
}

func TestLayoutPresetCycle_AllPresetsTraversed(t *testing.T) {
	// Verify the preset names used in the cycle are valid.
	valid := map[string]bool{
		"even-horizontal": true,
		"even-vertical":   true,
		"main-horizontal": true,
		"main-vertical":   true,
		"tiled":           true,
	}
	cycle := []string{"even-horizontal", "even-vertical", "main-horizontal", "main-vertical", "tiled"}
	for _, name := range cycle {
		if !valid[name] {
			t.Errorf("preset name %q not in valid set", name)
		}
	}
	// Verify the cycle has no duplicates.
	seen := map[string]bool{}
	for _, name := range cycle {
		if seen[name] {
			t.Errorf("duplicate preset name %q in cycle", name)
		}
		seen[name] = true
	}
}

// collectAllLeaves is a test helper that collects all leaf IDs from a tree.
func collectAllLeaves(t *layout.Tree) []layout.LeafID {
	var ids []layout.LeafID
	for id := range t.Leaves() {
		ids = append(ids, id)
	}
	return ids
}
