// Package log is dmux's logging infrastructure.
//
// dmux logs go to files, not stderr. The server's stdout and stderr
// are detached during daemonization; writing log output there is a
// black hole. The client's stdout is the user's real terminal —
// stderr is multiplexed into it, so writing log output to stderr
// would garble rendered pane content. Both processes write their
// logs to a file, and `-v` controls the verbosity threshold.
//
// # Backend
//
// Wraps log/slog from the stdlib. We take slog's Handler interface
// and wire in three concerns:
//
//   - File rotation at server restart (not in flight; no need).
//   - Structured context fields injected per-component (see below).
//   - Level control from command-line flags.
//
// No external dependencies. slog's TextHandler is the default.
//
// # Log locations
//
// Server: `$XDG_STATE_HOME/dmux/server-<socket-label>.log`, falling
// back to `~/.local/state/dmux/server-<socket-label>.log` on Unix
// and `%LOCALAPPDATA%\dmux\server-<socket-label>.log` on Windows.
// The socket-label segment lets users run multiple servers
// concurrently with distinct log streams.
//
// Client: `$XDG_STATE_HOME/dmux/client-<pid>.log` (same Windows
// path with client-<pid>.log). One file per client process; short
// lives are expected.
//
// Both processes create the containing directory at startup if
// missing. Failure to open the log file is a startup error.
//
// # Levels
//
//	slog.LevelError  serious failure; action almost always needed
//	slog.LevelWarn   unusual but recoverable (late DA2 reply, etc.)
//	slog.LevelInfo   normal lifecycle events (attach/detach, cmd exec)
//	slog.LevelDebug  verbose per-frame, per-event detail
//
// Command line:
//
//	dmux        -> Info
//	dmux -v     -> Debug
//	dmux -vv    -> Debug plus internal tracing (wasm calls, etc.)
//
// # Per-component context
//
// Each subsystem gets a tagged logger via `log.For(component)`:
//
//	srvLog := log.For("server")
//	clntLog := log.For("client", "client_id", cid)
//	paneLog := log.For("pane", "pane_id", pid, "session_id", sid)
//
// Every record from these loggers includes their tags. The Item
// interface (cmd.Item.Log()) returns a logger pre-tagged with
// command name and item id, so command-author code writes:
//
//	item.Log().Info("attached", "session", s.Name, "window", w.Index)
//
// # Structured fields, not format strings
//
// slog convention: facts are key-value, not printf. This is
// mandatory in dmux — every log record must use slog.Attr fields
// for variable data. Rationale: operators who want to grep for "all
// logs about pane 7" need structured `pane_id=7` tags, not
// `"... pane 7 ..."` embedded in arbitrary messages.
//
//	// Yes
//	log.Info("resize", "pane_id", p.ID, "cols", c, "rows", r)
//
//	// No
//	log.Info(fmt.Sprintf("resize pane %d to %dx%d", p.ID, c, r))
//
// # Interface
//
//	Open(cfg Config) error                         // called at process startup
//	Close() error                                  // called at process shutdown
//	For(component string, kv ...any) *slog.Logger  // tagged child logger
//	Default() *slog.Logger                         // the untagged root logger
//
//	type Config struct {
//	    Path  string            // resolved log file path
//	    Level slog.Level
//	}
//
// After Open, slog.Default() returns the configured logger; callers
// that don't need a tagged child can just use slog directly.
//
// # Error logging convention
//
// Log errors via the structured attr, not message interpolation:
//
//	log.Error("spawn failed", "err", err, "shell", shell)
//
// If the error wraps a typed error that carries context (e.g.
// cmd.TargetError), slog's default handler records the full chain
// via %+v — no special formatting required.
//
// # Goroutine safety
//
// slog.Logger is safe for concurrent use. No locking in package log.
//
// # Scope boundary
//
// No metrics, no tracing spans, no remote logging. If those land
// later, they're separate packages that hook slog's Handler.
package log
