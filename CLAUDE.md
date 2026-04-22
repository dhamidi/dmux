- use go doc to access read Go source and discover APIs
- follow diataxis.fr principles for writing documentation
- follow John Ousterhout's philosophy of software design: narrow interfaces, deep implementations.

## Build strategy: walking skeleton, not strict bottom-up

`docs/m1.md` lists an 8-layer build order. Follow the dependency
direction (nothing imports a higher layer) but not the idea of
finishing each layer before starting the next. Instead, reach a
runnable end-to-end `dmux` binary as early as possible with every
package stubbed to the thinnest implementation that makes the
top-level flow compile and run, then flesh out layer by layer
against a working skeleton.

Rationale: bottom-up-to-completion hides integration risk until
the last layer, where abstractions that looked clean in isolation
turn out not to compose. Walking skeleton pays a small duplication
cost (some packages get a throwaway stub first, then a real
implementation) in exchange for discovering interface mismatches
immediately and always having a binary that runs.

First skeleton checkpoint for M1: client dials server over a
socket, exchanges `Identify` + a stub `CommandResult`, both exit
cleanly. That proves proto + xio + socket + sockpath + a minimal
`cmd/dmux` compose before any pty, vt, pane, or rendering work
starts.

## Errors are part of the public API

Errors are not just log noise: callers `errors.Is` them for
control flow and `errors.As` them to pull context. Changing an
error's category, renaming a sentinel, or removing a struct field
breaks callers the same way removing an exported method does.
Design errors alongside the types they describe — before the
first call site lands, not after.

What makes a good error in this codebase:

1. **Sentinels for categories.** `var ErrFoo = errors.New("pkg:
   foo")` lets callers ask "is this the Foo case?" with
   `errors.Is`. One sentinel per distinct failure mode a caller
   might dispatch on — not one per `Error()` wording variant.

2. **Typed structs for detail.** When a caller might want to know
   *which* target was missing, *which* frame type was unknown, or
   *what* value was out of range, define a struct that carries
   those as fields and wraps the sentinel, so both `errors.Is` and
   `errors.As` work:

   ```go
   type FrameError struct {
       Op   string   // what was being attempted
       Type MsgType  // subject of the operation
       Err  error    // one of the package sentinels
   }
   func (e *FrameError) Error() string { ... }
   func (e *FrameError) Unwrap() error { return e.Err }
   ```

   The sentinel tells you the category; the struct lets you log or
   react to the specifics without parsing an error string.

3. **Wrap with %w across package boundaries.** When a lower-level
   error bubbles up through another package, use
   `fmt.Errorf("xio: read payload: %w", err)` so the chain stays
   intact for `errors.Is`/`errors.As` higher up.

4. **Stdlib string conventions.** Lowercase, no trailing
   punctuation, no stack traces. Errors compose into chains
   ("xio: write: proto: marshal Input: payload too large"); a
   stray capital letter or period in any segment looks wrong once
   concatenated.

5. **No exported fields of unexported types.** If a field's type
   is unexported, `errors.As` callers outside the package cannot
   use it. Prefer stable public types (strings, enums, IDs); keep
   the implementation detail inside `Error()`.

6. **No log-and-return.** Either handle the error (log + continue)
   or return it — not both. The final caller decides whether to
   log; double-logging clutters the record and obscures the real
   failure site.

7. **Error wording is de-facto API.** Once anyone greps logs or
   tests for a string, that string is load-bearing. Prefer adding
   structured fields over changing wording; call out wording
   changes in the commit message when unavoidable.
