package vt

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// wasmBinary is the libghostty-vt wasm32-freestanding build. The
// filename's hex suffix matches the upstream ghostty commit the build
// was cut from (see cabi/doc.go). Refreshing the wasm means refreshing
// both this file and cabi/ghostty/vt/ together.
//
//go:embed ghostty-vt-6e0b03.wasm
var wasmBinary []byte

// Runtime is a compiled libghostty-vt wasm module shared by all
// Terminals in a dmux server. Create one at server startup and pass
// it to NewTerminal whenever a pane needs a virtual terminal.
//
// A Runtime owns a wazero.Runtime plus one CompiledModule. Each
// Terminal gets its own Module instance so wasm linear memories do
// not alias across panes — which is what lets us honor the "one
// goroutine per Terminal" contract without a lock around wasm calls.
//
// Runtime.Close is idempotent. After Close, NewTerminal returns
// ErrClosed.
type Runtime struct {
	rt       wazero.Runtime
	compiled wazero.CompiledModule
	logger   *slog.Logger
	closed   bool
}

// Option configures a Runtime at construction.
type Option func(*runtimeConfig)

type runtimeConfig struct {
	logger *slog.Logger
}

// WithLogger routes the wasm module's env.log debug output through
// the given slog.Logger at DEBUG level. libghostty-vt only calls
// env.log on exceptional paths (assertion-like messages), so this is
// mostly noise — but when something goes wrong inside the VT it is
// the only signal we have.
func WithLogger(l *slog.Logger) Option {
	return func(c *runtimeConfig) { c.logger = l }
}

// NewRuntime compiles the embedded libghostty-vt wasm module and
// registers the single host import (env.log) the module requires.
//
// Production callers should reuse one Runtime for the lifetime of the
// server. Tests create a fresh Runtime per test.
func NewRuntime(ctx context.Context, opts ...Option) (*Runtime, error) {
	cfg := runtimeConfig{logger: slog.Default()}
	for _, opt := range opts {
		opt(&cfg)
	}

	rt := wazero.NewRuntime(ctx)

	_, err := rt.NewHostModuleBuilder("env").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, ptr, length uint32) {
			if cfg.logger == nil {
				return
			}
			msg, ok := mod.Memory().Read(ptr, length)
			if !ok {
				return
			}
			// Copy: libghostty reuses the buffer after the import returns.
			dup := make([]byte, len(msg))
			copy(dup, msg)
			cfg.logger.Debug("ghostty-vt", "msg", string(dup))
		}).
		Export("log").
		Instantiate(ctx)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, vtErr(OpNewRuntime, ErrInstantiate, err, "register env module")
	}

	compiled, err := rt.CompileModule(ctx, wasmBinary)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, vtErr(OpNewRuntime, ErrInstantiate, err, "compile module")
	}

	return &Runtime{rt: rt, compiled: compiled, logger: cfg.logger}, nil
}

// Close releases the wazero runtime and all in-flight module
// instances. Safe to call repeatedly.
func (r *Runtime) Close(ctx context.Context) error {
	if r.closed {
		return nil
	}
	r.closed = true
	if err := r.rt.Close(ctx); err != nil {
		return fmt.Errorf("vt: runtime close: %w", err)
	}
	return nil
}

// instantiate creates a fresh Module instance from the compiled
// module. Each Terminal gets its own instance (separate linear
// memory, separate stack). Called by NewTerminal.
func (r *Runtime) instantiate(ctx context.Context) (api.Module, error) {
	if r.closed {
		return nil, vtErr(OpNewTerminal, ErrClosed, nil, "runtime closed")
	}
	mod, err := r.rt.InstantiateModule(ctx, r.compiled, wazero.NewModuleConfig().
		WithName(""))
	if err != nil {
		return nil, vtErr(OpNewTerminal, ErrInstantiate, err, "instantiate module")
	}
	return mod, nil
}
