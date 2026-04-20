//go:build windows

package pty

import "errors"

// open creates a Windows ConPTY and starts cmd inside it.
// Requires Windows 10 version 1803 (build 17134) or later.
//
// This is a placeholder; the full ConPTY implementation (CreatePseudoConsole,
// pipe pairs, CreateProcessW + STARTUPINFOEX) is not yet written.
func open(cmd string, args []string, size Size) (PTY, error) {
	return nil, errors.New("pty: Windows ConPTY not yet implemented")
}
