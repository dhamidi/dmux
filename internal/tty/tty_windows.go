//go:build windows

package tty

import "os"

// TTY on Windows is a placeholder type so the package compiles.
// The real implementation based on GetConsoleScreenBufferInfo and
// the console control handler for window-resize notifications
// lands alongside the rest of the Windows M1 work; see doc.go.
type TTY struct{}

// Open on Windows returns ErrUnsupportedPlatform until the console
// implementation lands. The M1 acceptance criteria include Windows
// (criterion 6), so this stub exists only to keep the walking
// skeleton buildable on GOOS=windows while Unix is brought up.
func Open(stdin, stdout *os.File) (*TTY, error) {
	_ = stdin
	_ = stdout
	return nil, ttyErr(OpOpen, ErrUnsupportedPlatform, nil, "windows: console API not implemented yet")
}

func (t *TTY) Raw() error                  { return ErrUnsupportedPlatform }
func (t *TTY) Restore() error              { return ErrUnsupportedPlatform }
func (t *TTY) Size() (int, int, error)     { return 0, 0, ErrUnsupportedPlatform }
func (t *TTY) Read(p []byte) (int, error)  { return 0, ErrUnsupportedPlatform }
func (t *TTY) Write(p []byte) (int, error) { return 0, ErrUnsupportedPlatform }
func (t *TTY) Resize() <-chan ResizeEvent  { return nil }
func (t *TTY) EnableModes() error          { return ErrUnsupportedPlatform }
func (t *TTY) Close() error                { return nil }
