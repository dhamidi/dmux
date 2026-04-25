// Package script interprets dmux's command language line by line.
//
// One package, two callers: cmd/dmux runs scripts the user pipes on
// stdin or names on argv; internal/dmuxtest runs `.scenario` fixtures
// against an in-process server. Both reach the same per-line wire
// pattern through this package — Identify + CommandList for one
// argv per connection — so production and test share a single
// implementation of "execute a line of dmux".
//
// Surface:
//
//   - Tokenize splits a line into argv. Comments (`#` after optional
//     whitespace) and blank lines tokenize to a nil slice.
//   - RunLine opens one connection via the supplied Dialer, ships an
//     Identify + CommandList for one argv, reads the resulting
//     CommandResult, and returns. Connection closes on return; we
//     deliberately do not wait for an Exit frame, matching the
//     "command-only success = CommandResult{Ok} then close" contract
//     used by every non-attach command in the binary.
//   - Run drives an io.Reader of script lines through Tokenize and
//     RunLine, aborting on the first non-Ok result.
//
// Errors form a chain: a non-ok CommandResult turns into a
// *CommandError wrapping ErrCommandFailed; Run wraps that with a
// "script: line N:" prefix and the source identifier so callers can
// pinpoint the failing line without parsing strings.
package script
