// Package cabi is a documentation-only Go package that vendors the
// libghostty-vt public C ABI headers.
//
// The headers live under cabi/ghostty/vt/, preserving the upstream
// include/ directory structure so downstream readers can cross-reference
// comments and examples against the Ghostty source tree without needing
// a local clone. The Go side of this package contains no code; the
// headers are the documentation.
//
// These headers are kept here for two reasons:
//
//  1. The dmux vt integration calls the libghostty-vt wasm exports
//     directly through wazero, so the C ABI — struct layouts, enum
//     values, sized-struct conventions — is load-bearing at the
//     implementation level. Vendoring the headers means no runtime
//     dependency on a system install and no build-time dependency on
//     a ghostty checkout.
//
//  2. Upstream evolves; the ABI is stable but not frozen. Vendoring
//     pins the exact header set that matches ghostty-vt-6e0b03.wasm
//     and makes ABI changes visible in a diff when the wasm is
//     refreshed.
//
// Source: https://github.com/ghostty-org/ghostty — see the upstream
// AGENTS.md for the wasm build recipe. The vendored headers came from
// include/ghostty/vt/ at commit 6e0b0311e (matching the wasm filename
// suffix).
package cabi
