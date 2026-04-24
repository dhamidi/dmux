// Package client implements the "client" ensemble command.
//
// # Synopsis
//
//	client spawn [-F profile] [-x cols] [-y rows] <name>
//	client kill  <name>
//	client at    <name> <bytes>
//
// The ensemble dispatches on its first positional argument: "spawn"
// creates a new in-process client and records its opaque reference in
// the user option @client/<name>; "kill" reads that option, tears the
// client down, and unsets the option; "at" reads the same option and
// injects bytes into the named client's input stream so the server
// sees them as Input frames exactly as if that client's tty had
// emitted them.
//
// # Byte injection
//
// The bytes argument to "at" is a Go-quoted string literal: the
// ensemble wraps it in double quotes and feeds it through
// strconv.Unquote so escapes (`\n`, `\xNN`, `\uNNNN`) follow the
// same rules as Go source literals. A malformed escape surfaces as a
// *args.ParseError naming the "bytes" positional. An @client/<name>
// ref that no longer resolves to a live client (Inject wraps
// ErrStaleClient) unsets the option before surfacing the error, so
// stale bookkeeping does not block the next "spawn" under the same
// name.
//
// # Rationale
//
// The server needs a way for external drivers (tests, agents, hook
// scripts) to address synthetic clients by a stable name rather than
// by an opaque runtime handle. User options (`@client/<name>`) act as
// a symbol table: callers look references up by name from anywhere
// scoped options are visible, which keeps scenario scripts readable
// without introducing a second namespace.
//
// # Stale-reference tolerance
//
// `client kill` must not fail when the referenced client is already
// gone — crashes, races, and bookkeeping drift all produce stale
// refs. When ClientManager.Kill returns an error that wraps
// cmd.ErrStaleClient, the ensemble treats it as success after
// unsetting the option. The user option is always unset before the
// Kill call so repeated invocations converge.
//
// # Scope boundary
//
// Profile validation, tty-size negotiation, and the "wait until the
// spawned client finishes handshake" semantics live inside
// ClientManager. The command only plumbs flags → ClientManager →
// options table.
package client
