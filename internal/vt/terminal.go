package vt

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// Grid is the reified state of a terminal screen at one instant.
// Cells is indexed [row][col].
type Grid struct {
	Cols  int
	Rows  int
	Cells [][]Cell
}

// Cell is a single grid cell. M1 only tracks the codepoint and the
// wide flag; style, color, hyperlink, and grapheme extensions land
// when termout needs them.
type Cell struct {
	Rune rune
	Wide CellWide
}

// CellWide mirrors GhosttyCellWide: narrow (width 1), wide (width 2),
// spacer-tail (second half of a wide cell, do not render), and
// spacer-head (end-of-soft-wrap padding before a wide cell).
type CellWide uint8

const (
	CellNarrow     CellWide = 0
	CellWideWide   CellWide = 1
	CellSpacerTail CellWide = 2
	CellSpacerHead CellWide = 3
)

// Cursor is the current live-screen cursor position. X and Y are
// zero-indexed cell coordinates inside the active area (not the
// scrollback).
type Cursor struct {
	X, Y    int
	Visible bool
}

// Terminal data kinds we query (subset of GhosttyTerminalData).
const (
	termDataCursorX       uint32 = 3
	termDataCursorY       uint32 = 4
	termDataCursorVisible uint32 = 7
)

// Render-state data / row / cell kinds we query.
const (
	renderDataRowIterator uint32 = 4

	rowDataCells uint32 = 3

	cellDataRaw uint32 = 1
)

// Cell-get data kinds (GhosttyCellData).
const (
	cellGetCodepoint uint32 = 1
	cellGetWide      uint32 = 3
)

// Terminal wraps one GhosttyTerminal plus its render pipeline
// (render state, row iterator, row cells) inside a dedicated wazero
// Module instance.
//
// Not safe for concurrent use. Each Terminal is owned by exactly one
// goroutine — the pane goroutine that created it. All methods must
// be called from that goroutine.
type Terminal struct {
	rt     *Runtime
	ctx    context.Context
	mod    api.Module
	memory api.Memory

	// Allocator helpers (ghostty_wasm_*). Their by-len variants
	// cover the general-purpose byte scratch needs.
	fnAllocU8Array api.Function
	fnFreeU8Array  api.Function

	// Terminal lifecycle.
	fnTermNew     api.Function
	fnTermFree    api.Function
	fnTermResize  api.Function
	fnTermVTWrite api.Function
	fnTermGet     api.Function

	// Render state.
	fnStateNew    api.Function
	fnStateFree   api.Function
	fnStateUpdate api.Function
	fnStateGet    api.Function

	// Row iterator (wired to state via state_get(DATA_ROW_ITERATOR)).
	fnIterNew  api.Function
	fnIterFree api.Function
	fnIterNext api.Function
	fnRowGet   api.Function

	// Row cells iterator (wired to iter via row_get(ROW_DATA_CELLS)).
	fnCellsNew    api.Function
	fnCellsFree   api.Function
	fnCellsNext   api.Function
	fnCellsSelect api.Function
	fnCellsGet    api.Function

	// GhosttyCell (u64) accessor.
	fnCellGet api.Function

	// Handles into wasm-side opaque types. These are the values the
	// wasm exports return through their *_new out-pointers.
	termH  uint32
	stateH uint32
	iterH  uint32
	cellsH uint32

	// Slots in wasm memory that permanently hold iterH/cellsH.
	// render_state_get(ROW_ITERATOR) and row_get(ROW_DATA_CELLS)
	// both take a pointer-to-handle as their "out" argument (they
	// read the existing handle and wire it to the current
	// state/row). Keeping the slots resident avoids reallocating
	// on every Snapshot.
	iterSlot  uint32
	cellsSlot uint32

	// Persistent scratch buffers. Allocated at NewTerminal, freed
	// on Close. Kept resident so we do not round-trip the wasm
	// allocator on every Feed/Snapshot.
	scratch4 uint32 // 4-byte slot: u16/bool reads + alloc-out handles
	scratch8 uint32 // 8-byte slot: GhosttyCell u64 reads

	// Feed buffer grows on demand to fit the largest Write to date.
	feedBuf    uint32
	feedBufLen uint32

	closed bool
}

// NewTerminal instantiates a fresh libghostty-vt wasm module, creates
// a terminal of the given size, and wires up the render state used
// by Snapshot.
//
// Dimensions must be positive. The caller keeps the ctx alive for
// the Terminal's lifetime; wasm calls use it verbatim.
func (r *Runtime) NewTerminal(ctx context.Context, cols, rows int) (*Terminal, error) {
	if cols <= 0 || rows <= 0 {
		return nil, vtErr(OpNewTerminal, nil, nil, fmt.Sprintf("bad dims cols=%d rows=%d", cols, rows))
	}

	mod, err := r.instantiate(ctx)
	if err != nil {
		return nil, err
	}

	t := &Terminal{
		rt:     r,
		ctx:    ctx,
		mod:    mod,
		memory: mod.Memory(),
	}

	// Resolve every export up front. A missing export means the
	// embedded wasm is out of sync with this Go file — fail hard
	// with a clear name rather than NPE'ing on first Call.
	lookups := []struct {
		out  *api.Function
		name string
	}{
		{&t.fnAllocU8Array, "ghostty_wasm_alloc_u8_array"},
		{&t.fnFreeU8Array, "ghostty_wasm_free_u8_array"},
		{&t.fnTermNew, "ghostty_terminal_new"},
		{&t.fnTermFree, "ghostty_terminal_free"},
		{&t.fnTermResize, "ghostty_terminal_resize"},
		{&t.fnTermVTWrite, "ghostty_terminal_vt_write"},
		{&t.fnTermGet, "ghostty_terminal_get"},
		{&t.fnStateNew, "ghostty_render_state_new"},
		{&t.fnStateFree, "ghostty_render_state_free"},
		{&t.fnStateUpdate, "ghostty_render_state_update"},
		{&t.fnStateGet, "ghostty_render_state_get"},
		{&t.fnIterNew, "ghostty_render_state_row_iterator_new"},
		{&t.fnIterFree, "ghostty_render_state_row_iterator_free"},
		{&t.fnIterNext, "ghostty_render_state_row_iterator_next"},
		{&t.fnRowGet, "ghostty_render_state_row_get"},
		{&t.fnCellsNew, "ghostty_render_state_row_cells_new"},
		{&t.fnCellsFree, "ghostty_render_state_row_cells_free"},
		{&t.fnCellsNext, "ghostty_render_state_row_cells_next"},
		{&t.fnCellsSelect, "ghostty_render_state_row_cells_select"},
		{&t.fnCellsGet, "ghostty_render_state_row_cells_get"},
		{&t.fnCellGet, "ghostty_cell_get"},
	}
	for _, l := range lookups {
		*l.out = mod.ExportedFunction(l.name)
		if *l.out == nil {
			_ = mod.Close(ctx)
			return nil, vtErr(OpNewTerminal, ErrInstantiate, nil, "missing export "+l.name)
		}
	}

	if err := t.init(ctx, cols, rows); err != nil {
		_ = mod.Close(ctx)
		return nil, err
	}
	return t, nil
}

// init runs all the one-shot wasm setup: persistent scratch buffers,
// terminal handle, render state + iterators. On any failure the
// partial resources it allocated are rolled back.
func (t *Terminal) init(ctx context.Context, cols, rows int) error {
	var rollback []func()
	fail := func(err error) error {
		for i := len(rollback) - 1; i >= 0; i-- {
			rollback[i]()
		}
		return err
	}

	// Persistent 4-byte scratch (also doubles as the handle-out
	// slot while setting up, then stays resident for codepoint
	// reads during Snapshot).
	s4, err := t.alloc(ctx, 4)
	if err != nil {
		return fail(err)
	}
	t.scratch4 = s4
	rollback = append(rollback, func() { _ = t.free(ctx, t.scratch4, 4); t.scratch4 = 0 })

	s8, err := t.alloc(ctx, 8)
	if err != nil {
		return fail(err)
	}
	t.scratch8 = s8
	rollback = append(rollback, func() { _ = t.free(ctx, t.scratch8, 8); t.scratch8 = 0 })

	// Terminal: fill GhosttyTerminalOptions {u16 cols, u16 rows,
	// size_t max_scrollback} and call ghostty_terminal_new.
	optsPtr, err := t.alloc(ctx, 8)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = t.free(ctx, optsPtr, 8) }()
	if !t.writeU16(optsPtr+0, uint16(cols)) ||
		!t.writeU16(optsPtr+2, uint16(rows)) ||
		!t.writeU32(optsPtr+4, 10000 /* max_scrollback lines */) {
		return fail(vtErr(OpNewTerminal, nil, nil, "write options"))
	}

	// ghostty_terminal_new puts its out-pointer in the MIDDLE
	// (allocator, out_terminal, options-by-pointer), not at the end
	// like the other _new functions, so it cannot share
	// callHandleOut's tail-append convention.
	termOut, err := t.alloc(ctx, 4)
	if err != nil {
		return fail(err)
	}
	if !t.writeU32(termOut, 0) {
		_ = t.free(ctx, termOut, 4)
		return fail(vtErr(OpNewTerminal, nil, nil, "clear termOut"))
	}
	ret, err := t.fnTermNew.Call(ctx, 0 /* allocator */, uint64(termOut), uint64(optsPtr))
	if err != nil {
		_ = t.free(ctx, termOut, 4)
		return fail(vtErr(OpNewTerminal, nil, err, "terminal_new"))
	}
	if code := CabiResult(int32(ret[0])); code != CabiSuccess {
		_ = t.free(ctx, termOut, 4)
		return fail(cabiErr(OpNewTerminal, "ghostty_terminal_new", code))
	}
	termH, ok := t.readU32(termOut)
	_ = t.free(ctx, termOut, 4)
	if !ok || termH == 0 {
		return fail(vtErr(OpNewTerminal, ErrOutOfMemory, nil, "null terminal handle"))
	}
	t.termH = termH
	rollback = append(rollback, func() {
		_, _ = t.fnTermFree.Call(ctx, uint64(t.termH))
		t.termH = 0
	})

	stateSlot, stateH, err := t.newHandleSlot(ctx, t.fnStateNew, "ghostty_render_state_new")
	if err != nil {
		return fail(err)
	}
	_ = t.free(ctx, stateSlot, 4) // state handle is used by value; slot not needed.
	t.stateH = stateH
	rollback = append(rollback, func() {
		_, _ = t.fnStateFree.Call(ctx, uint64(t.stateH))
		t.stateH = 0
	})

	// iter and cells: allocate a persistent 4-byte slot that holds
	// the handle (the libghostty_*_new writes to it) and stays
	// resident so Snapshot can pass &slot to render_state_get and
	// row_get as the "rebind this iterator" out argument.
	iterSlot, iterH, err := t.newHandleSlot(ctx, t.fnIterNew, "ghostty_render_state_row_iterator_new")
	if err != nil {
		return fail(err)
	}
	t.iterSlot = iterSlot
	t.iterH = iterH
	rollback = append(rollback, func() {
		_, _ = t.fnIterFree.Call(ctx, uint64(t.iterH))
		_ = t.free(ctx, t.iterSlot, 4)
		t.iterH, t.iterSlot = 0, 0
	})

	cellsSlot, cellsH, err := t.newHandleSlot(ctx, t.fnCellsNew, "ghostty_render_state_row_cells_new")
	if err != nil {
		return fail(err)
	}
	t.cellsSlot = cellsSlot
	t.cellsH = cellsH
	rollback = append(rollback, func() {
		_, _ = t.fnCellsFree.Call(ctx, uint64(t.cellsH))
		_ = t.free(ctx, t.cellsSlot, 4)
		t.cellsH, t.cellsSlot = 0, 0
	})

	return nil
}

// newHandleSlot allocates a 4-byte wasm memory slot, invokes a
// constructor of the form `fn(allocator, out_handle) -> GhosttyResult`
// (the two-arg shape used by render_state_new, render_state_row_iterator_new,
// and render_state_row_cells_new), and returns the slot plus the
// read-back handle. The slot is kept for the caller to pass back into
// bind-style getters (e.g. render_state_get DATA_ROW_ITERATOR).
func (t *Terminal) newHandleSlot(ctx context.Context, fn api.Function, fnName string) (uint32, uint32, error) {
	slot, err := t.alloc(ctx, 4)
	if err != nil {
		return 0, 0, err
	}
	if !t.writeU32(slot, 0) {
		_ = t.free(ctx, slot, 4)
		return 0, 0, vtErr(OpNewTerminal, nil, nil, fnName+": clear slot")
	}
	ret, err := fn.Call(ctx, 0 /* allocator */, uint64(slot))
	if err != nil {
		_ = t.free(ctx, slot, 4)
		return 0, 0, vtErr(OpNewTerminal, nil, err, fnName)
	}
	if code := CabiResult(int32(ret[0])); code != CabiSuccess {
		_ = t.free(ctx, slot, 4)
		return 0, 0, cabiErr(OpNewTerminal, fnName, code)
	}
	h, ok := t.readU32(slot)
	if !ok || h == 0 {
		_ = t.free(ctx, slot, 4)
		return 0, 0, vtErr(OpNewTerminal, ErrOutOfMemory, nil, fnName+": null handle")
	}
	return slot, h, nil
}

// alloc allocates len bytes inside the wasm linear memory via the
// wasm-specific helper. Returns a pointer (wasm-memory offset).
func (t *Terminal) alloc(ctx context.Context, length uint32) (uint32, error) {
	ret, err := t.fnAllocU8Array.Call(ctx, uint64(length))
	if err != nil {
		return 0, vtErr(OpNewTerminal, nil, err, "alloc_u8_array")
	}
	p := uint32(ret[0])
	if p == 0 {
		return 0, vtErr(OpNewTerminal, ErrOutOfMemory, nil, fmt.Sprintf("alloc %d bytes", length))
	}
	return p, nil
}

func (t *Terminal) free(ctx context.Context, ptr, length uint32) error {
	if ptr == 0 {
		return nil
	}
	_, err := t.fnFreeU8Array.Call(ctx, uint64(ptr), uint64(length))
	return err
}

func (t *Terminal) readU16(ptr uint32) (uint16, bool) {
	b, ok := t.memory.Read(ptr, 2)
	if !ok {
		return 0, false
	}
	return binary.LittleEndian.Uint16(b), true
}

func (t *Terminal) readU32(ptr uint32) (uint32, bool) {
	return t.memory.ReadUint32Le(ptr)
}

func (t *Terminal) readU64(ptr uint32) (uint64, bool) {
	return t.memory.ReadUint64Le(ptr)
}

func (t *Terminal) writeU16(ptr uint32, v uint16) bool {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], v)
	return t.memory.Write(ptr, buf[:])
}

func (t *Terminal) writeU32(ptr uint32, v uint32) bool {
	return t.memory.WriteUint32Le(ptr, v)
}

// Feed pushes VT-encoded bytes into the terminal. This is the real
// pipeline equivalent of "PTY wrote this; parse it." It never fails
// on malformed input — libghostty-vt explicitly documents that
// ghostty_terminal_vt_write treats its input as untrusted.
func (t *Terminal) Feed(b []byte) error {
	if t.closed {
		return vtErr(OpFeed, ErrClosed, nil, "")
	}
	if len(b) == 0 {
		return nil
	}
	if err := t.ensureFeedBuf(t.ctx, uint32(len(b))); err != nil {
		return err
	}
	if !t.memory.Write(t.feedBuf, b) {
		return vtErr(OpFeed, nil, nil, "copy into wasm memory")
	}
	_, err := t.fnTermVTWrite.Call(t.ctx, uint64(t.termH), uint64(t.feedBuf), uint64(len(b)))
	if err != nil {
		return vtErr(OpFeed, nil, err, "vt_write")
	}
	return nil
}

// ensureFeedBuf grows the Feed scratch buffer if it cannot hold n
// bytes. The buffer is held for the Terminal's lifetime to avoid
// an alloc+free round-trip on every pty chunk.
func (t *Terminal) ensureFeedBuf(ctx context.Context, n uint32) error {
	if n <= t.feedBufLen {
		return nil
	}
	if t.feedBuf != 0 {
		if err := t.free(ctx, t.feedBuf, t.feedBufLen); err != nil {
			return vtErr(OpFeed, nil, err, "free old feed buffer")
		}
	}
	// Grow geometrically so chatty panes do not thrash the allocator.
	grow := t.feedBufLen * 2
	if grow < n {
		grow = n
	}
	if grow < 4096 {
		grow = 4096
	}
	p, err := t.alloc(ctx, grow)
	if err != nil {
		return err
	}
	t.feedBuf = p
	t.feedBufLen = grow
	return nil
}

// Resize forwards to ghostty_terminal_resize. Pixel dimensions are
// passed as 8x16 placeholders: dmux does not track real cell pixel
// sizes yet, and libghostty only uses them for image protocols and
// size reports we do not exercise in M1.
func (t *Terminal) Resize(cols, rows int) error {
	if t.closed {
		return vtErr(OpResize, ErrClosed, nil, "")
	}
	if cols <= 0 || rows <= 0 {
		return vtErr(OpResize, nil, nil, fmt.Sprintf("bad dims cols=%d rows=%d", cols, rows))
	}
	ret, err := t.fnTermResize.Call(t.ctx,
		uint64(t.termH),
		uint64(uint16(cols)),
		uint64(uint16(rows)),
		uint64(8),
		uint64(16))
	if err != nil {
		return vtErr(OpResize, nil, err, "terminal_resize")
	}
	if code := CabiResult(int32(ret[0])); code != CabiSuccess {
		return cabiErr(OpResize, "ghostty_terminal_resize", code)
	}
	return nil
}

// Cursor reads the live cursor position + visibility from the
// terminal. It does NOT snapshot the render state; callers that want
// both a grid and a cursor in one consistent view should call
// Snapshot first (which consumes dirty state), then Cursor.
func (t *Terminal) Cursor() (Cursor, error) {
	if t.closed {
		return Cursor{}, vtErr(OpCursor, ErrClosed, nil, "")
	}
	x, err := t.termGetU16(termDataCursorX, "CURSOR_X")
	if err != nil {
		return Cursor{}, err
	}
	y, err := t.termGetU16(termDataCursorY, "CURSOR_Y")
	if err != nil {
		return Cursor{}, err
	}
	visible, err := t.termGetBool(termDataCursorVisible, "CURSOR_VISIBLE")
	if err != nil {
		return Cursor{}, err
	}
	return Cursor{X: int(x), Y: int(y), Visible: visible}, nil
}

func (t *Terminal) termGetU16(kind uint32, fn string) (uint16, error) {
	ret, err := t.fnTermGet.Call(t.ctx, uint64(t.termH), uint64(kind), uint64(t.scratch4))
	if err != nil {
		return 0, vtErr(OpCursor, nil, err, "terminal_get "+fn)
	}
	if code := CabiResult(int32(ret[0])); code != CabiSuccess {
		return 0, cabiErr(OpCursor, "ghostty_terminal_get:"+fn, code)
	}
	v, ok := t.readU16(t.scratch4)
	if !ok {
		return 0, vtErr(OpCursor, nil, nil, "read "+fn)
	}
	return v, nil
}

func (t *Terminal) termGetBool(kind uint32, fn string) (bool, error) {
	// _Bool is 1 byte in Zig's C ABI on wasm, but the terminal_get
	// docs specify `bool *`, which the compiler lays out as 1 byte.
	// Reading 4 bytes and masking to the low byte is safe because
	// scratch4 is our own memory.
	ret, err := t.fnTermGet.Call(t.ctx, uint64(t.termH), uint64(kind), uint64(t.scratch4))
	if err != nil {
		return false, vtErr(OpCursor, nil, err, "terminal_get "+fn)
	}
	if code := CabiResult(int32(ret[0])); code != CabiSuccess {
		return false, cabiErr(OpCursor, "ghostty_terminal_get:"+fn, code)
	}
	b, ok := t.memory.ReadByte(t.scratch4)
	if !ok {
		return false, vtErr(OpCursor, nil, nil, "read "+fn)
	}
	return b != 0, nil
}

// Snapshot reifies the live screen into a Go Grid. It consumes the
// terminal's dirty state as a side-effect of
// ghostty_render_state_update — a subsequent Snapshot on an
// unchanged terminal will still report the full grid, but dirty bits
// will be false everywhere (which we do not expose from M1).
func (t *Terminal) Snapshot() (Grid, error) {
	if t.closed {
		return Grid{}, vtErr(OpSnapshot, ErrClosed, nil, "")
	}

	// 1. Update the render state from the terminal.
	ret, err := t.fnStateUpdate.Call(t.ctx, uint64(t.stateH), uint64(t.termH))
	if err != nil {
		return Grid{}, vtErr(OpSnapshot, nil, err, "render_state_update")
	}
	if code := CabiResult(int32(ret[0])); code != CabiSuccess {
		return Grid{}, cabiErr(OpSnapshot, "ghostty_render_state_update", code)
	}

	// 2. Wire the row iterator into the state. render_state_get
	// takes a pointer-to-handle for its out argument; it reads the
	// existing iterator handle from the slot and binds it.
	ret, err = t.fnStateGet.Call(t.ctx, uint64(t.stateH), uint64(renderDataRowIterator), uint64(t.iterSlot))
	if err != nil {
		return Grid{}, vtErr(OpSnapshot, nil, err, "render_state_get ROW_ITERATOR")
	}
	if code := CabiResult(int32(ret[0])); code != CabiSuccess {
		return Grid{}, cabiErr(OpSnapshot, "ghostty_render_state_get:ROW_ITERATOR", code)
	}

	// 3. Walk rows. We do not trust a pre-declared row count;
	// iterate until next() returns false so future resizes just
	// work.
	var grid Grid
	for {
		nextRet, err := t.fnIterNext.Call(t.ctx, uint64(t.iterH))
		if err != nil {
			return Grid{}, vtErr(OpSnapshot, nil, err, "row_iterator_next")
		}
		if nextRet[0] == 0 {
			break
		}

		// 4. Wire the cells iterator into this row. Same
		// pointer-to-handle pattern as the state-level bind above.
		ret, err := t.fnRowGet.Call(t.ctx, uint64(t.iterH), uint64(rowDataCells), uint64(t.cellsSlot))
		if err != nil {
			return Grid{}, vtErr(OpSnapshot, nil, err, "row_get ROW_DATA_CELLS")
		}
		if code := CabiResult(int32(ret[0])); code != CabiSuccess {
			return Grid{}, cabiErr(OpSnapshot, "ghostty_render_state_row_get:ROW_DATA_CELLS", code)
		}

		var row []Cell
		for {
			cellsNext, err := t.fnCellsNext.Call(t.ctx, uint64(t.cellsH))
			if err != nil {
				return Grid{}, vtErr(OpSnapshot, nil, err, "row_cells_next")
			}
			if cellsNext[0] == 0 {
				break
			}
			cell, err := t.readCurrentCell()
			if err != nil {
				return Grid{}, err
			}
			row = append(row, cell)
		}
		grid.Cells = append(grid.Cells, row)
	}

	grid.Rows = len(grid.Cells)
	if grid.Rows > 0 {
		grid.Cols = len(grid.Cells[0])
	}
	return grid, nil
}

// readCurrentCell reads the currently-positioned cell from the
// cells iterator: RAW -> GhosttyCell u64, then decode codepoint +
// wide flag from that handle.
func (t *Terminal) readCurrentCell() (Cell, error) {
	// RAW -> u64
	ret, err := t.fnCellsGet.Call(t.ctx, uint64(t.cellsH), uint64(cellDataRaw), uint64(t.scratch8))
	if err != nil {
		return Cell{}, vtErr(OpSnapshot, nil, err, "row_cells_get RAW")
	}
	if code := CabiResult(int32(ret[0])); code != CabiSuccess {
		return Cell{}, cabiErr(OpSnapshot, "ghostty_render_state_row_cells_get:RAW", code)
	}
	cellVal, ok := t.readU64(t.scratch8)
	if !ok {
		return Cell{}, vtErr(OpSnapshot, nil, nil, "read cell u64")
	}

	// Codepoint.
	ret, err = t.fnCellGet.Call(t.ctx, cellVal, uint64(cellGetCodepoint), uint64(t.scratch4))
	if err != nil {
		return Cell{}, vtErr(OpSnapshot, nil, err, "cell_get CODEPOINT")
	}
	if code := CabiResult(int32(ret[0])); code != CabiSuccess {
		return Cell{}, cabiErr(OpSnapshot, "ghostty_cell_get:CODEPOINT", code)
	}
	cp, ok := t.readU32(t.scratch4)
	if !ok {
		return Cell{}, vtErr(OpSnapshot, nil, nil, "read codepoint")
	}

	// Wide flag — an int-backed enum; read as u32 and truncate.
	ret, err = t.fnCellGet.Call(t.ctx, cellVal, uint64(cellGetWide), uint64(t.scratch4))
	if err != nil {
		return Cell{}, vtErr(OpSnapshot, nil, err, "cell_get WIDE")
	}
	if code := CabiResult(int32(ret[0])); code != CabiSuccess {
		return Cell{}, cabiErr(OpSnapshot, "ghostty_cell_get:WIDE", code)
	}
	wide, ok := t.readU32(t.scratch4)
	if !ok {
		return Cell{}, vtErr(OpSnapshot, nil, nil, "read wide")
	}

	return Cell{Rune: rune(cp), Wide: CellWide(wide)}, nil
}

// Close frees the wasm-side handles and the per-Terminal module
// instance. Idempotent.
func (t *Terminal) Close() error {
	if t.closed {
		return nil
	}
	t.closed = true
	ctx := t.ctx

	// Free in reverse-creation order. Errors are best-effort —
	// the module is about to be Closed anyway.
	if t.cellsH != 0 {
		_, _ = t.fnCellsFree.Call(ctx, uint64(t.cellsH))
	}
	if t.iterH != 0 {
		_, _ = t.fnIterFree.Call(ctx, uint64(t.iterH))
	}
	if t.stateH != 0 {
		_, _ = t.fnStateFree.Call(ctx, uint64(t.stateH))
	}
	if t.termH != 0 {
		_, _ = t.fnTermFree.Call(ctx, uint64(t.termH))
	}
	if t.feedBuf != 0 {
		_ = t.free(ctx, t.feedBuf, t.feedBufLen)
	}
	if t.scratch8 != 0 {
		_ = t.free(ctx, t.scratch8, 8)
	}
	if t.scratch4 != 0 {
		_ = t.free(ctx, t.scratch4, 4)
	}

	if err := t.mod.Close(ctx); err != nil {
		return vtErr(OpClose, nil, err, "module close")
	}
	return nil
}
