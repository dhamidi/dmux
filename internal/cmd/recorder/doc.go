// Package recorder implements the recorder ensemble command.
//
// # Synopsis
//
//	recorder set-level <normal|debug>
//
// # Rationale
//
// The event recorder in internal/record has a verbosity knob
// (LevelNormal vs LevelDebug) that gates high-volume diagnostic
// emissions like per-byte vt.feed, per-iteration loop.iter, and
// per-frame socket.read. Being able to flip that knob at runtime
// matters for two audiences:
//
//   - Online debugging. An operator inspecting a running server wants
//     to promote the recorder to Debug for the duration of a
//     reproducible misbehaviour, then drop it back to Normal so the
//     event stream stays readable. Restarting the process to raise
//     the level is not an option.
//   - Hooks and plugins. Scenario runners, inspector-style tooling,
//     and user-defined hooks need a command-line surface for the same
//     capability so they can bracket a sensitive operation with
//     level changes without linking against internal/record directly.
//
// The recorder ensemble is where that surface lives. The record
// package stays transport-agnostic — it only exposes SetLevel and
// the Level constants — and the command package owns the
// string-to-Level mapping.
//
// # Subcommands
//
//   - set-level <normal|debug>
//     Change the current recorder's emission level. No-op when no
//     recorder is open (production defaults and scenario harnesses
//     both tolerate that).
//
// # Scope boundary
//
// set-level is the only knob the ensemble exposes today. Future
// subcommands (open / close / snapshot) may join it, but each will
// land alongside the code path that needs it; the package is a
// container for recorder-shaped operations, not a pre-built
// inventory.
package recorder
