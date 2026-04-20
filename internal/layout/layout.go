package layout

import (
	"fmt"
	"iter"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// LeafID is an opaque identifier for a leaf pane in the layout tree.
type LeafID int

// Direction indicates whether a split is horizontal or vertical.
type Direction int

const (
	// Horizontal splits panes side by side (left/right).
	Horizontal Direction = iota
	// Vertical stacks panes top to bottom.
	Vertical
)

// Edge identifies a border of a pane.
type Edge int

const (
	EdgeTop    Edge = iota
	EdgeBottom Edge = iota
	EdgeLeft   Edge = iota
	EdgeRight  Edge = iota
)

// Rect describes the position and size of a pane in character cells.
type Rect struct {
	X, Y, Width, Height int
}

// BorderID identifies the shared border between two adjacent panes. PaneID is
// one of the panes; Edge is that pane's edge along which the border lies.
type BorderID struct {
	PaneID LeafID
	Edge   Edge
}

// PaneAt returns the LeafID of the pane whose rectangle contains the point
// (col, row). Returns (0, false) if no pane covers that point.
func PaneAt(t *Tree, col, row int) (LeafID, bool) {
	for id := range t.Leaves() {
		r := t.Rect(id)
		if r.Width == 0 || r.Height == 0 {
			continue
		}
		if col >= r.X && col < r.X+r.Width && row >= r.Y && row < r.Y+r.Height {
			return id, true
		}
	}
	return 0, false
}

// BorderAt returns the BorderID of the border (pane edge) that passes through
// (col, row). A border is the 1-cell gap at the right or bottom edge of a
// pane that is shared with an adjacent pane.
func BorderAt(t *Tree, col, row int) (*BorderID, bool) {
	for id := range t.Leaves() {
		r := t.Rect(id)
		if r.Width == 0 || r.Height == 0 {
			continue
		}
		if col == r.X+r.Width && row >= r.Y && row < r.Y+r.Height {
			return &BorderID{PaneID: id, Edge: EdgeRight}, true
		}
		if row == r.Y+r.Height && col >= r.X && col < r.X+r.Width {
			return &BorderID{PaneID: id, Edge: EdgeBottom}, true
		}
	}
	return nil, false
}

// Preset names a common layout arrangement.
type Preset int

const (
	PresetEvenHorizontal Preset = iota // all panes in a single row
	PresetEvenVertical                 // all panes in a single column
	PresetMainHorizontal               // one large pane on top, rest below
	PresetMainVertical                 // one large pane on left, rest on right
	PresetTiled                        // grid arrangement
)

// node is the recursive building block of the layout tree.
type node struct {
	// exactly one of leaf or split is non-nil
	leaf  *leafNode
	split *splitNode
}

type leafNode struct {
	id LeafID
}

type splitNode struct {
	dir      Direction
	children []*node
	// sizes holds the relative weight of each child (sum need not be 1).
	sizes []float64
}

// Tree is the root of the pane layout binary tree.
type Tree struct {
	cols, rows int
	root       *node
	nextID     LeafID
	zoomed     LeafID // 0 means not zoomed; valid IDs start at 1
	zoomActive bool
}

// New creates a new Tree with dimensions cols×rows containing a single leaf
// identified by first.
func New(cols, rows int, first LeafID) *Tree {
	t := &Tree{
		cols:   cols,
		rows:   rows,
		nextID: first + 1,
	}
	t.root = &node{leaf: &leafNode{id: first}}
	return t
}

// newLeafID allocates the next LeafID.
func (t *Tree) newLeafID() LeafID {
	id := t.nextID
	t.nextID++
	return id
}

// Split divides the pane identified by leaf along dir, inserting a new pane
// after it. It returns the LeafID of the newly created pane.
func (t *Tree) Split(leaf LeafID, dir Direction) LeafID {
	newID := t.newLeafID()
	t.root = splitNode_(t.root, leaf, dir, newID)
	return newID
}

// splitNode_ recurses through the tree and splits the target leaf.
func splitNode_(n *node, target LeafID, dir Direction, newID LeafID) *node {
	if n.leaf != nil {
		if n.leaf.id != target {
			return n
		}
		// Replace this leaf with a split containing the old leaf and a new leaf.
		return &node{
			split: &splitNode{
				dir: dir,
				children: []*node{
					{leaf: &leafNode{id: target}},
					{leaf: &leafNode{id: newID}},
				},
				sizes: []float64{0.5, 0.5},
			},
		}
	}
	if n.split != nil {
		s := n.split
		// Check if any direct child is the target leaf so we can add a sibling.
		for i, child := range s.children {
			if child.leaf != nil && child.leaf.id == target && s.dir == dir {
				// Append within the same split direction.
				newChildren := make([]*node, len(s.children)+1)
				copy(newChildren, s.children[:i+1])
				newChildren[i+1] = &node{leaf: &leafNode{id: newID}}
				copy(newChildren[i+2:], s.children[i+1:])
				w := 1.0 / float64(len(newChildren))
				newSizes := make([]float64, len(newChildren))
				for j := range newSizes {
					newSizes[j] = w
				}
				return &node{
					split: &splitNode{
						dir:      s.dir,
						children: newChildren,
						sizes:    newSizes,
					},
				}
			}
		}
		// Recurse into children.
		newChildren := make([]*node, len(s.children))
		for i, child := range s.children {
			newChildren[i] = splitNode_(child, target, dir, newID)
		}
		return &node{
			split: &splitNode{
				dir:      s.dir,
				children: newChildren,
				sizes:    s.sizes,
			},
		}
	}
	return n
}

// Close removes the pane identified by leaf from the tree.
func (t *Tree) Close(leaf LeafID) {
	if t.zoomActive && t.zoomed == leaf {
		t.zoomActive = false
		t.zoomed = 0
	}
	result := closeNode(t.root, leaf)
	if result != nil {
		t.root = result
	}
}

// closeNode removes the target leaf. Returns nil if this node itself should be
// removed (i.e. it was the leaf).
func closeNode(n *node, target LeafID) *node {
	if n.leaf != nil {
		if n.leaf.id == target {
			return nil
		}
		return n
	}
	if n.split != nil {
		s := n.split
		newChildren := make([]*node, 0, len(s.children))
		for _, child := range s.children {
			result := closeNode(child, target)
			if result != nil {
				newChildren = append(newChildren, result)
			}
		}
		if len(newChildren) == 0 {
			return nil
		}
		if len(newChildren) == 1 {
			return newChildren[0]
		}
		// Re-normalise sizes.
		w := 1.0 / float64(len(newChildren))
		newSizes := make([]float64, len(newChildren))
		for i := range newSizes {
			newSizes[i] = w
		}
		return &node{
			split: &splitNode{
				dir:      s.dir,
				children: newChildren,
				sizes:    newSizes,
			},
		}
	}
	return n
}

// Resize updates the overall window dimensions.
func (t *Tree) Resize(cols, rows int) {
	t.cols = cols
	t.rows = rows
}

// MoveBorder shifts a border of the given pane by delta cells.
// delta > 0 moves the border away from the pane centre; delta < 0 moves it
// toward the centre. Only the nearest sibling is affected.
func (t *Tree) MoveBorder(leaf LeafID, edge Edge, delta int) {
	moveBorderInNode(t.root, leaf, edge, delta)
}

// moveBorderInNode recursively finds the leaf and adjusts sibling sizes.
func moveBorderInNode(n *node, target LeafID, edge Edge, delta int) bool {
	if n.leaf != nil {
		return n.leaf.id == target
	}
	if n.split == nil {
		return false
	}
	s := n.split
	for i, child := range s.children {
		if !moveBorderInNode(child, target, edge, delta) {
			continue
		}
		// Found the subtree containing target.
		switch {
		case (edge == EdgeRight || edge == EdgeBottom) && i < len(s.children)-1:
			s.sizes[i] += float64(delta)
			s.sizes[i+1] -= float64(delta)
			if s.sizes[i] < 1 {
				s.sizes[i] = 1
			}
			if s.sizes[i+1] < 1 {
				s.sizes[i+1] = 1
			}
		case (edge == EdgeLeft || edge == EdgeTop) && i > 0:
			s.sizes[i-1] -= float64(delta)
			s.sizes[i] += float64(delta)
			if s.sizes[i-1] < 1 {
				s.sizes[i-1] = 1
			}
			if s.sizes[i] < 1 {
				s.sizes[i] = 1
			}
		}
		return true
	}
	return false
}

// Rect returns the rectangle allocated to the pane identified by leaf.
// If zoom is active and leaf is not the zoomed pane, a zero Rect is returned.
func (t *Tree) Rect(leaf LeafID) Rect {
	if t.zoomActive {
		if leaf == t.zoomed {
			return Rect{X: 0, Y: 0, Width: t.cols, Height: t.rows}
		}
		return Rect{}
	}
	r, _ := rectInNode(t.root, leaf, Rect{X: 0, Y: 0, Width: t.cols, Height: t.rows})
	return r
}

// rectInNode descends the tree computing each child's rectangle.
func rectInNode(n *node, target LeafID, available Rect) (Rect, bool) {
	if n.leaf != nil {
		if n.leaf.id == target {
			return available, true
		}
		return Rect{}, false
	}
	if n.split == nil {
		return Rect{}, false
	}
	s := n.split
	total := 0.0
	for _, sz := range s.sizes {
		total += sz
	}
	offset := 0
	for i, child := range s.children {
		frac := s.sizes[i] / total
		var childRect Rect
		if s.dir == Horizontal {
			w := int(math.Round(frac * float64(available.Width)))
			if i == len(s.children)-1 {
				w = available.Width - offset
			}
			childRect = Rect{X: available.X + offset, Y: available.Y, Width: w, Height: available.Height}
			offset += w
		} else {
			h := int(math.Round(frac * float64(available.Height)))
			if i == len(s.children)-1 {
				h = available.Height - offset
			}
			childRect = Rect{X: available.X, Y: available.Y + offset, Width: available.Width, Height: h}
			offset += h
		}
		if r, ok := rectInNode(child, target, childRect); ok {
			return r, true
		}
	}
	return Rect{}, false
}

// Leaves returns an iterator over all LeafIDs in the tree, in order.
func (t *Tree) Leaves() iter.Seq[LeafID] {
	return func(yield func(LeafID) bool) {
		leavesInNode(t.root, yield)
	}
}

func leavesInNode(n *node, yield func(LeafID) bool) bool {
	if n.leaf != nil {
		return yield(n.leaf.id)
	}
	if n.split != nil {
		for _, child := range n.split.children {
			if !leavesInNode(child, yield) {
				return false
			}
		}
	}
	return true
}

// Zoom temporarily maximises leaf to the full window size.
func (t *Tree) Zoom(leaf LeafID) {
	t.zoomed = leaf
	t.zoomActive = true
}

// Unzoom reverts a previous Zoom call.
func (t *Tree) Unzoom() {
	t.zoomed = 0
	t.zoomActive = false
}

// Cols returns the width of the window in character cells.
// IsZoomed reports whether a pane in this tree is currently zoomed.
func (t *Tree) IsZoomed() bool { return t.zoomActive }

// ZoomedLeaf returns the LeafID of the currently zoomed pane.
// Returns 0 when no pane is zoomed (t.IsZoomed() == false).
func (t *Tree) ZoomedLeaf() LeafID { return t.zoomed }

func (t *Tree) Cols() int { return t.cols }

// Rows returns the height of the window in character cells.
func (t *Tree) Rows() int { return t.rows }

// SwapLeaves exchanges the positions of two leaves in the tree without
// changing the split structure or sizes. If either ID is not present in the
// tree, SwapLeaves is a no-op.
func (t *Tree) SwapLeaves(a, b LeafID) {
	ids := collectLeaves(t.root)
	ai, bi := -1, -1
	for i, id := range ids {
		if id == a {
			ai = i
		} else if id == b {
			bi = i
		}
	}
	if ai < 0 || bi < 0 || ai == bi {
		return
	}
	ids[ai], ids[bi] = ids[bi], ids[ai]
	pos := 0
	assignLeavesInNode(t.root, ids, &pos)
}

// RotateLeaves rotates the leaf IDs within the tree without changing the
// split structure or sizes. If forward is true, pane 0 moves to the position
// of pane 1, pane 1 to position 2, etc.; the last pane wraps to position 0.
// If forward is false, the rotation is reversed.
func (t *Tree) RotateLeaves(forward bool) {
	ids := collectLeaves(t.root)
	n := len(ids)
	if n <= 1 {
		return
	}
	rotated := make([]LeafID, n)
	if forward {
		// pane 0 → pos 1, pane n-1 → pos 0
		copy(rotated[1:], ids[:n-1])
		rotated[0] = ids[n-1]
	} else {
		// pane 1 → pos 0, pane 0 → pos n-1
		copy(rotated[:n-1], ids[1:])
		rotated[n-1] = ids[0]
	}
	pos := 0
	assignLeavesInNode(t.root, rotated, &pos)
}

// assignLeavesInNode replaces leaf IDs in DFS order from ids.
func assignLeavesInNode(n *node, ids []LeafID, pos *int) {
	if n.leaf != nil {
		n.leaf.id = ids[*pos]
		*pos++
		return
	}
	if n.split != nil {
		for _, child := range n.split.children {
			assignLeavesInNode(child, ids, pos)
		}
	}
}

// ApplyPreset rearranges the tree according to preset p.
func (t *Tree) ApplyPreset(p Preset) {
	t.ApplyPresetSized(p, 0)
}

// ApplyPresetSized is like ApplyPreset but accepts an explicit mainSize for
// PresetMainHorizontal (height in rows) and PresetMainVertical (width in
// columns). For other presets mainSize is ignored. A mainSize of 0 falls back
// to an even 50/50 split.
func (t *Tree) ApplyPresetSized(p Preset, mainSize int) {
	leaves := collectLeaves(t.root)
	if len(leaves) == 0 {
		return
	}
	switch p {
	case PresetEvenHorizontal:
		t.root = buildFlat(leaves, Horizontal)
	case PresetEvenVertical:
		t.root = buildFlat(leaves, Vertical)
	case PresetMainHorizontal:
		t.root = buildMainSized(leaves, Vertical, mainSize, t.rows)
	case PresetMainVertical:
		t.root = buildMainSized(leaves, Horizontal, mainSize, t.cols)
	case PresetTiled:
		t.root = buildTiled(leaves)
	}
}

func collectLeaves(n *node) []LeafID {
	if n.leaf != nil {
		return []LeafID{n.leaf.id}
	}
	var ids []LeafID
	if n.split != nil {
		for _, child := range n.split.children {
			ids = append(ids, collectLeaves(child)...)
		}
	}
	return ids
}

func buildFlat(leaves []LeafID, dir Direction) *node {
	if len(leaves) == 1 {
		return &node{leaf: &leafNode{id: leaves[0]}}
	}
	children := make([]*node, len(leaves))
	sizes := make([]float64, len(leaves))
	w := 1.0 / float64(len(leaves))
	for i, id := range leaves {
		children[i] = &node{leaf: &leafNode{id: id}}
		sizes[i] = w
	}
	return &node{split: &splitNode{dir: dir, children: children, sizes: sizes}}
}

func buildMain(leaves []LeafID, mainDir Direction) *node {
	return buildMainSized(leaves, mainDir, 0, 0)
}

func buildMainSized(leaves []LeafID, mainDir Direction, mainSize, total int) *node {
	if len(leaves) == 1 {
		return &node{leaf: &leafNode{id: leaves[0]}}
	}
	main := &node{leaf: &leafNode{id: leaves[0]}}
	rest := buildFlat(leaves[1:], otherDir(mainDir))
	mainFrac, restFrac := 0.5, 0.5
	if mainSize > 0 && total > mainSize {
		mainFrac = float64(mainSize)
		restFrac = float64(total - mainSize)
	}
	return &node{
		split: &splitNode{
			dir:      mainDir,
			children: []*node{main, rest},
			sizes:    []float64{mainFrac, restFrac},
		},
	}
}

func buildTiled(leaves []LeafID) *node {
	n := len(leaves)
	if n == 0 {
		return nil
	}
	if n == 1 {
		return &node{leaf: &leafNode{id: leaves[0]}}
	}
	cols := int(math.Ceil(math.Sqrt(float64(n))))
	rows := int(math.Ceil(float64(n) / float64(cols)))
	var rowNodes []*node
	for r := 0; r < rows; r++ {
		start := r * cols
		end := start + cols
		if end > n {
			end = n
		}
		rowNodes = append(rowNodes, buildFlat(leaves[start:end], Horizontal))
	}
	return buildFlatNodes(rowNodes, Vertical)
}

func buildFlatNodes(nodes []*node, dir Direction) *node {
	if len(nodes) == 1 {
		return nodes[0]
	}
	sizes := make([]float64, len(nodes))
	w := 1.0 / float64(len(nodes))
	for i := range sizes {
		sizes[i] = w
	}
	return &node{split: &splitNode{dir: dir, children: nodes, sizes: sizes}}
}

func otherDir(d Direction) Direction {
	if d == Horizontal {
		return Vertical
	}
	return Horizontal
}

// Marshal serialises the tree in a tmux-compatible layout string.
// Format: "<checksum>,<WxH>,<X>,<Y>[{...}|[...]]"
func (t *Tree) Marshal() string {
	body := marshalNode(t.root, 0, 0, t.cols, t.rows)
	checksum := layoutChecksum(body)
	return fmt.Sprintf("%04x,%s", checksum, body)
}

func marshalNode(n *node, x, y, w, h int) string {
	if n.leaf != nil {
		return fmt.Sprintf("%dx%d,%d,%d,%d", w, h, x, y, int(n.leaf.id))
	}
	if n.split == nil {
		return ""
	}
	s := n.split
	total := 0.0
	for _, sz := range s.sizes {
		total += sz
	}
	var parts []string
	offset := 0
	for i, child := range s.children {
		frac := s.sizes[i] / total
		var cw, ch, cx, cy int
		if s.dir == Horizontal {
			cw = int(math.Round(frac * float64(w)))
			if i == len(s.children)-1 {
				cw = w - offset
			}
			ch = h
			cx = x + offset
			cy = y
			offset += cw
		} else {
			cw = w
			ch = int(math.Round(frac * float64(h)))
			if i == len(s.children)-1 {
				ch = h - offset
			}
			cx = x
			cy = y + offset
			offset += ch
		}
		parts = append(parts, marshalNode(child, cx, cy, cw, ch))
	}
	if s.dir == Horizontal {
		return fmt.Sprintf("%dx%d,%d,%d{%s}", w, h, x, y, strings.Join(parts, ","))
	}
	return fmt.Sprintf("%dx%d,%d,%d[%s]", w, h, x, y, strings.Join(parts, ","))
}

func layoutChecksum(s string) uint16 {
	var csum uint16
	for i := 0; i < len(s); i++ {
		csum = (csum >> 1) | ((csum & 1) << 15)
		csum += uint16(s[i])
	}
	return csum
}

// Unmarshal parses a tmux-compatible layout string and returns a Tree.
// The cols and rows are taken from the root dimensions in the string.
func Unmarshal(s string) (*Tree, error) {
	// Strip optional leading checksum "xxxx,"
	if len(s) > 5 && s[4] == ',' {
		s = s[5:]
	}
	root, nextID, err := parseNode(s)
	if err != nil {
		return nil, err
	}
	// Extract top-level dims.
	dims, err := parseTopDims(s)
	if err != nil {
		return nil, err
	}
	return &Tree{
		cols:   dims[0],
		rows:   dims[1],
		root:   root,
		nextID: nextID + 1,
	}, nil
}

var dimRe = regexp.MustCompile(`^(\d+)x(\d+),(\d+),(\d+)`)

func parseTopDims(s string) ([2]int, error) {
	m := dimRe.FindStringSubmatch(s)
	if m == nil {
		return [2]int{}, fmt.Errorf("layout: invalid format %q", s)
	}
	w, _ := strconv.Atoi(m[1])
	h, _ := strconv.Atoi(m[2])
	return [2]int{w, h}, nil
}

func parseNode(s string) (*node, LeafID, error) {
	m := dimRe.FindStringSubmatch(s)
	if m == nil {
		return nil, 0, fmt.Errorf("layout: cannot parse node %q", s)
	}
	// Advance past "WxH,X,Y"
	rest := s[len(m[0]):]

	var maxID LeafID

	if rest == "" {
		// Shouldn't happen without pane id, treat as leaf 0.
		return &node{leaf: &leafNode{id: 0}}, 0, nil
	}

	switch rest[0] {
	case '{', '[':
		dir := Horizontal
		closeChar := byte('}')
		if rest[0] == '[' {
			dir = Vertical
			closeChar = ']'
		}
		inner := rest[1 : len(rest)-1]
		if len(inner) == 0 || rest[len(rest)-1] != closeChar {
			return nil, 0, fmt.Errorf("layout: mismatched brackets in %q", s)
		}
		children, ids, err := splitChildren(inner, dir)
		if err != nil {
			return nil, 0, err
		}
		for _, id := range ids {
			if id > maxID {
				maxID = id
			}
		}
		return &node{split: &splitNode{
			dir:      dir,
			children: children,
			sizes:    evenSizes(len(children)),
		}}, maxID, nil
	case ',':
		// Leaf: ",<id>"
		idStr := rest[1:]
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return nil, 0, fmt.Errorf("layout: bad pane id %q", rest)
		}
		return &node{leaf: &leafNode{id: LeafID(id)}}, LeafID(id), nil
	default:
		return nil, 0, fmt.Errorf("layout: unexpected character %q in %q", rest[0], s)
	}
}

func evenSizes(n int) []float64 {
	sizes := make([]float64, n)
	w := 1.0 / float64(n)
	for i := range sizes {
		sizes[i] = w
	}
	return sizes
}

// splitChildren splits a comma-separated list of child node strings, respecting
// nested brackets.
func splitChildren(s string, dir Direction) ([]*node, []LeafID, error) {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ',':
			if depth == 0 {
				// Check if this comma separates children or is part of WxH,X,Y
				// Children are separated at top-level commas that follow a complete node.
				// We split after "WxH,X,Y" or "WxH,X,Y,id" patterns.
				sub := s[start:i]
				if isCompleteNode(sub) {
					parts = append(parts, sub)
					start = i + 1
				}
			}
		}
	}
	parts = append(parts, s[start:])

	var children []*node
	var ids []LeafID
	for _, part := range parts {
		n, maxID, err := parseNode(part)
		if err != nil {
			return nil, nil, err
		}
		children = append(children, n)
		ids = append(ids, maxID)
	}
	return children, ids, nil
}

// isCompleteNode reports whether s is a complete node string.
// A complete leaf node matches: WxH,X,Y,id
// A complete split node ends with } or ].
func isCompleteNode(s string) bool {
	if len(s) == 0 {
		return false
	}
	last := s[len(s)-1]
	if last == '}' || last == ']' {
		return true
	}
	// Leaf: must match WxH,X,Y,id  — four comma-separated fields after the dims.
	parts := strings.Split(s, ",")
	return len(parts) == 4
}
