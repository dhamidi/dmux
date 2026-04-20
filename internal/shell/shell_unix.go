//go:build !windows

package shell

func defaultShell(env func(string) (string, bool), exists func(string) bool) string {
	if s, ok := env("SHELL"); ok && s != "" {
		return s
	}
	if exists("/bin/sh") {
		return "/bin/sh"
	}
	return "sh"
}
