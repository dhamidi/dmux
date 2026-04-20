//go:build windows

package shell

func defaultShell(env func(string) (string, bool), exists func(string) bool) string {
	if s, ok := env("COMSPEC"); ok && s != "" {
		return s
	}
	for _, p := range []string{
		`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
		`C:\Program Files\PowerShell\7\pwsh.exe`,
	} {
		if exists(p) {
			return p
		}
	}
	return "cmd.exe"
}
