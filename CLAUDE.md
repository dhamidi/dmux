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

## Implementing commands

Commands live in `internal/cmd/<name>/` and share a small set of
conventions. New commands should follow them so the queue, the
argv parser, and the option-table integration stay uniform.

1. **One package per command; zero-struct implementation.** Each
   command exports a `Name` constant and registers a zero-sized
   `command struct{}` in `init()`:

   ```go
   const Name = "new-session"
   type command struct{}
   func (command) Name() string { return Name }
   func (command) Exec(item cmd.Item, argv []string) cmd.Result { ... }
   func init() { cmd.Register(command{}) }
   ```

   The struct is stateless on purpose: everything mutable lives on
   `cmd.Item`. Registration happens at init-time, so importing the
   package (usually via a blank import in `cmd/dmux/main.go`) is
   what wires the command into the registry.

2. **Parse argv with `internal/cmd/args`, not raw `flag`.** The
   `args` package bundles a `flag.FlagSet` with an ordered list of
   typed positionals. Declare dashed flags with `s.String` /
   `s.Bool` / `s.Int` and positionals with `s.StringArg` /
   `s.BoolArg` / `s.IntArg`. `s.Parse(argv[1:])` fills both; extra
   tokens are available via `s.Rest()`. Parse failures come back
   as `*args.ParseError` so call sites that care about phase
   ("flags" vs "positional") can `errors.As` into it. Required
   positionals are checked by the command after Parse — treat a
   missing handle as a `*args.ParseError` with the positional's
   name so diagnostics are uniform with flag errors.

3. **TCL-style ensembles when a command has subcommands.** When a
   command groups related operations (`client spawn`, `client
   kill`), implement one registered command whose Exec dispatches
   on `argv[1]`. Unknown subcommands return a `*args.ParseError`
   (Phase: `"positional"`, Name: `"subcommand"`, Value: the bad
   token) so callers get the same structured diagnostic as any
   other parse failure. Keep each subcommand's flag set local to
   its handler; do not share a `args.Set` across branches.

4. **Extending `cmd.Item` is how commands get new capabilities.**
   When a command needs a facility the Item interface does not
   expose (option table access, client manager, hook firing,
   logging sink), add a narrow accessor to `cmd.Item` and a
   matching method on `serverItem`. Define the capability as its
   own interface (`ClientManager`, `SessionLookup`) so fakes in
   tests can implement just the surface they need. Commands read
   through `item.Options()` / `item.Clients()` / etc. — never
   import server internals directly.

5. **User options (`@<name>`) are the symbol table for named
   handles.** When a command needs to remember a handle across
   invocations (spawned clients, AI agents, pending prompts), write
   the opaque reference to a user option (`@client/<name>`,
   `@agent/<name>`) as a `String` value and read it back on later
   invocations. This unifies scenario scripts with production use:
   both reach the handle by name through the normal options
   scope-chain, no separate namespace. `options.IsUserOption`
   identifies the prefix; unset user options read as the empty
   string, which is the signal for "no such handle yet".

6. **Tolerate stale references.** Any command that acts on a
   previously-stored handle must tolerate the underlying resource
   already being gone. The convention is: the capability interface
   (e.g. `ClientManager.Kill`) returns an error wrapping
   `cmd.ErrStaleClient` (or the package's equivalent sentinel) when
   the ref no longer resolves; the calling command uses
   `errors.Is` to treat that as success. Unset the user option
   *before* calling the underlying tear-down so repeated
   invocations converge regardless of outcome.

7. **Roll back on partial failure.** When a command performs two
   steps that must both succeed (spawn a client, then record its
   ref), undo the first if the second fails. The `client spawn`
   path is the template: if `Options().Set` fails after a
   successful `Clients().Spawn`, call `Clients().Kill(ref)` before
   returning. Leaking an untracked resource is worse than returning
   the original error — the caller can retry, but only if the
   world is back to a known state.

8. **Test with fakes that implement only the surface the command
   uses.** Each command's test file declares a local `fakeItem`
   that satisfies `cmd.Item` by returning `nil` from every method
   it does not exercise, plus a dedicated fake for each capability
   it does (e.g. a `fakeClients` that tracks spawn/kill calls in a
   map). Keep the fakes in the `_test.go` file — they are
   per-command scaffolding, not shared infrastructure.
