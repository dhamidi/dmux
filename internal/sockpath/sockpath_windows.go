//go:build windows

package sockpath

import "path/filepath"

// resolveDefault implements the Windows path layout:
// %LOCALAPPDATA%\dmux\<label>. No ownership or mode check is done;
// NTFS ACLs and default %LOCALAPPDATA% permissions already restrict
// access to the current user.
func resolveDefault(getenv func(string) string, label string) (string, error) {
	localappdata := getenv("LOCALAPPDATA")
	if localappdata == "" {
		return "", ErrNoLocalAppData
	}
	return filepath.Join(localappdata, "dmux", label), nil
}
