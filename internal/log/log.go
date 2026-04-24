package log

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// Sentinel errors. Callers use errors.Is to dispatch on category.
var (
	// ErrAlreadyOpen is returned when Open is called while the package
	// already holds an open log file.
	ErrAlreadyOpen = errors.New("log: already open")

	// ErrPathUnresolved is returned when a path-resolution helper
	// cannot determine any candidate directory (no $XDG_STATE_HOME,
	// no $HOME on Unix; no %LOCALAPPDATA% on Windows).
	ErrPathUnresolved = errors.New("log: path unresolved")

	// ErrInvalidLabel is returned when a path-resolution helper is
	// called with an empty socket label.
	ErrInvalidLabel = errors.New("log: invalid label")

	// ErrEmptyPath is returned by Open when cfg.Path is the empty
	// string.
	ErrEmptyPath = errors.New("log: empty path")
)

// Config configures the package-level logger installed by Open.
type Config struct {
	// Path is the resolved log file path. The parent directory is
	// created with mode 0700 if it does not exist; the file is
	// opened with O_CREATE|O_WRONLY|O_APPEND and mode 0600.
	Path string
	// Level is the minimum record level emitted by the handler.
	Level slog.Level
}

// Package-level state is guarded by a single mutex. slog.Logger is
// itself concurrent-safe; the mutex only serializes Open/Close and
// reads of the current logger pointer against those writes.
var (
	mu         sync.Mutex
	file       *os.File
	current    *slog.Logger
	prevLogger *slog.Logger
)

// Open opens the log file described by cfg, installs a slog.TextHandler
// at cfg.Level in front of it, and makes the resulting logger the
// package default as well as slog.Default. Returns ErrAlreadyOpen if
// called while the package already holds an open file.
func Open(cfg Config) error {
	mu.Lock()
	defer mu.Unlock()

	if file != nil {
		return ErrAlreadyOpen
	}

	if cfg.Path == "" {
		return ErrEmptyPath
	}

	dir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("log: mkdir %s: %w", dir, err)
	}

	f, err := os.OpenFile(cfg.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("log: open %s: %w", cfg.Path, err)
	}

	handler := slog.NewTextHandler(f, &slog.HandlerOptions{Level: cfg.Level})
	logger := slog.New(handler)

	// Capture whatever slog.Default was before we installed ours so
	// Close can restore it.
	prevLogger = slog.Default()
	file = f
	current = logger
	slog.SetDefault(logger)
	return nil
}

// Close flushes and closes the underlying log file, restores the
// previous slog.Default, and clears the package-level logger. It is
// idempotent: calling Close on a closed (or never-opened) logger
// returns nil.
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if file == nil {
		return nil
	}

	f := file
	prev := prevLogger
	file = nil
	current = nil
	prevLogger = nil

	if prev != nil {
		slog.SetDefault(prev)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("log: close: %w", err)
	}
	return nil
}

// Default returns the logger installed by Open. If Open has not been
// called (or the package has been closed), it returns slog.Default().
func Default() *slog.Logger {
	mu.Lock()
	defer mu.Unlock()
	if current != nil {
		return current
	}
	return slog.Default()
}

// For returns a child logger tagged with the given component name and
// any additional key/value pairs. It never panics: before Open it
// still returns a tagged logger, backed by slog.Default().
func For(component string, kv ...any) *slog.Logger {
	base := Default()
	attrs := make([]any, 0, 2+len(kv))
	attrs = append(attrs, "component", component)
	attrs = append(attrs, kv...)
	return base.With(attrs...)
}
