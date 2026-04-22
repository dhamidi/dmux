//go:build windows

package pty

import "context"

// PTY on Windows is a placeholder type so the package compiles.
// The real ConPTY-based implementation lands alongside other
// Windows-specific M1 work; see doc.go.
type PTY struct{}

// Spawn on Windows returns ErrUnsupportedPlatform until the ConPTY
// implementation lands. The M1 acceptance criteria include Windows
// (criterion 6), so this stub exists only to keep the walking
// skeleton buildable on GOOS=windows while Unix is brought up.
func Spawn(ctx context.Context, cfg Config) (*PTY, error) {
	_ = ctx
	_ = cfg
	return nil, spawnErr(OpStart, ErrUnsupportedPlatform, nil, "windows: ConPTY not implemented yet")
}

func (p *PTY) Read(b []byte) (int, error)     { return 0, ErrUnsupportedPlatform }
func (p *PTY) Write(b []byte) (int, error)    { return 0, ErrUnsupportedPlatform }
func (p *PTY) Resize(cols, rows int) error    { return ErrUnsupportedPlatform }
func (p *PTY) Signal(sig Signal) error        { return ErrUnsupportedPlatform }
func (p *PTY) Wait() (ExitStatus, error)      { return ExitStatus{}, ErrUnsupportedPlatform }
func (p *PTY) Close() error                   { return nil }
