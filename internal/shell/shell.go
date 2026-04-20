package shell

// Default returns the shell executable path to use when the user has not
// specified one.
//
// env is called to look up environment variables (e.g. os.LookupEnv).
// exists is called to check whether a filesystem path is present and
// executable (e.g. a thin wrapper around os.Stat).
//
// No direct OS or environment calls are made inside this package; all
// system access is performed through the supplied functions.
func Default(env func(key string) (string, bool), exists func(path string) bool) string {
	return defaultShell(env, exists)
}
