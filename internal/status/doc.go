// Package status renders the status line(s) into cells.
//
// # Accepted interfaces
//
// All dependencies are accepted as narrow interfaces so that callers
// can test status rendering with stubs and no real session or format
// engine is ever required.
//
// [Context] — provides variable resolution during format string expansion:
//
//	type Context interface {
//	    Lookup(key string) (string, bool)
//	}
//
// Any type with a matching Lookup method satisfies Context, including
// format.MapContext (a plain map[string]string) and the session types
// (session.Session, session.Client, session.Window, …) which implement
// the richer format.Context.
//
// [Expander] — expands #{…} format template strings:
//
//	type Expander interface {
//	    Expand(template string, ctx Context) string
//	}
//
// The concrete format.Expander satisfies this interface via an adapter
// that wraps a status.Context with a no-op Children method to satisfy
// format.Context. Expansion errors are swallowed; an erroneous directive
// expands to an empty string.
//
// [Options] — supplies the format strings used to configure each line:
//
//	type Options interface {
//	    StatusLeft() string
//	    StatusRight() string
//	    StatusFormat(n int) string   // 0-based line index
//	    StatusLineCount() int
//	}
//
// The concrete session options.Store satisfies this interface via a thin
// adapter that calls GetString("status-left"), GetString("status-right"),
// and GetString("status-format-N").
//
// # Constructor
//
//	New(expander Expander, ctx Context, opts Options) *StatusLine
//
// New creates a StatusLine. No other concrete types are needed; callers
// supply fakes during testing.
//
// # Exported methods
//
// [StatusLine.Render] produces exactly width cells for the primary status
// line (status-format-0). It falls back to joining StatusLeft() and
// StatusRight() with a space when status-format-0 is empty. Render
// satisfies the render.StatusLine interface.
//
// [StatusLine.Lines] produces one [Line] per configured status format,
// expanding each against ctx. It returns nil when StatusLineCount is 0.
//
// # Style ranges
//
// Expanded strings may contain embedded #[fg=color,bg=color,…] style
// markers. This package strips those markers before mapping runes to
// cells. Colour and attribute information is not stored in [Cell] at
// this layer; callers that need styled output should extend Cell or
// use a richer cell type upstream.
//
// # In isolation
//
// Renderable against a stubbed Context and Expander. Tests in this
// package verify particular format strings produce the right cells
// without ever booting a real server or pane.
//
// # Non-goals
//
// Not drawn here. render.Compose places the Lines. Not evaluated on a
// timer here; the server loop calls Render on its redraw cadence.
package status
