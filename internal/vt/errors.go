package vt

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors. Callers use errors.Is to dispatch on category; the
// concrete error returned by this package is usually a *VTError
// wrapping one of these plus structured fields that errors.As can
// pull out.
var (
	// ErrClosed is returned by any method when Close has already run.
	ErrClosed = errors.New("vt: closed")

	// ErrInstantiate is returned by NewRuntime or NewTerminal when
	// wazero fails to compile or instantiate the embedded wasm
	// module. The underlying wazero error is reachable via Unwrap.
	ErrInstantiate = errors.New("vt: instantiate failed")

	// ErrCabi is returned when a libghostty-vt C ABI call reports a
	// non-success GhosttyResult. The specific result code is carried
	// on VTError.Code so callers can dispatch without parsing text.
	ErrCabi = errors.New("vt: cabi error")

	// ErrOutOfMemory is returned when a wasm-side allocation call
	// (ghostty_alloc, ghostty_wasm_alloc_*) returns a NULL pointer.
	// It is distinct from ErrCabi because the wasm allocator
	// signals failure by returning 0 rather than a result code.
	ErrOutOfMemory = errors.New("vt: out of memory")
)

// Op names the method that was executing when an error surfaced.
// Carried on VTError so callers can log or dispatch on the failing
// step without parsing Error().
type Op string

const (
	OpNewRuntime  Op = "new-runtime"
	OpNewTerminal Op = "new-terminal"
	OpFeed        Op = "feed"
	OpResize      Op = "resize"
	OpSnapshot    Op = "snapshot"
	OpCursor      Op = "cursor"
	OpClose       Op = "close"
)

// CabiResult mirrors the libghostty-vt GhosttyResult enum. Zero is
// success; negative values encode distinct failure modes.
type CabiResult int32

const (
	CabiSuccess     CabiResult = 0
	CabiOutOfMemory CabiResult = -1
	CabiInvalidArg  CabiResult = -2
	CabiOutOfSpace  CabiResult = -3
	CabiNoValue     CabiResult = -4
)

func (r CabiResult) String() string {
	switch r {
	case CabiSuccess:
		return "success"
	case CabiOutOfMemory:
		return "out-of-memory"
	case CabiInvalidArg:
		return "invalid-value"
	case CabiOutOfSpace:
		return "out-of-space"
	case CabiNoValue:
		return "no-value"
	default:
		return fmt.Sprintf("unknown(%d)", int32(r))
	}
}

// VTError is the concrete error type returned by this package. It
// carries the operation, an optional libghostty result code, a detail
// string, and an underlying wrapped error.
//
// Category dispatch is done through the package sentinels via
// errors.Is; structured inspection (Op, Code) via errors.As.
type VTError struct {
	Op       Op
	Sentinel error // one of the package sentinels, or nil
	Code     CabiResult
	Fn       string // libghostty export name, when applicable
	Detail   string
	Err      error // wrapped cause, or nil
}

func (e *VTError) Error() string {
	var b strings.Builder
	b.WriteString("vt: ")
	b.WriteString(string(e.Op))
	if e.Fn != "" {
		b.WriteString(": ")
		b.WriteString(e.Fn)
	}
	if e.Code != CabiSuccess && e.Sentinel == ErrCabi {
		b.WriteString(": ")
		b.WriteString(e.Code.String())
	}
	if e.Sentinel != nil && e.Sentinel != ErrCabi {
		b.WriteString(": ")
		b.WriteString(strings.TrimPrefix(e.Sentinel.Error(), "vt: "))
	}
	if e.Detail != "" {
		b.WriteString(": ")
		b.WriteString(e.Detail)
	}
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

func (e *VTError) Unwrap() error { return e.Err }

func (e *VTError) Is(target error) bool {
	return e.Sentinel != nil && e.Sentinel == target
}

func vtErr(op Op, sentinel error, err error, detail string) error {
	return &VTError{Op: op, Sentinel: sentinel, Detail: detail, Err: err}
}

func cabiErr(op Op, fn string, code CabiResult) error {
	return &VTError{Op: op, Sentinel: ErrCabi, Fn: fn, Code: code}
}
