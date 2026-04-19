// Package status renders the status line(s) into cells.
//
// # Boundary
//
//	func Render(in Input) []Line
//
//	type Input struct {
//	    Width    int           // client terminal width in cells
//	    Templates Templates    // status-left, status-right, status-format-N
//	    FormatEnv format.Env   // Context + ShellRunner + clock for #(...) etc.
//	    Theme    Theme         // default fg/bg if a range omits one
//	}
//
//	type Templates struct {
//	    Left, Right string
//	    Lines       []string
//	}
//
// status takes only interface-typed inputs. Production wires the
// session.Client / session.Server in by constructing the Templates
// (option lookups), the format.Env (Context implementation backed by
// session, ShellRunner backed by job), and the Width (Client.Size).
// status itself imports neither session nor command nor job.
//
// The expanded strings may contain embedded #[fg=color,bg=color] style
// markers that this package parses into cell attributes — tmux calls
// these "style ranges."
//
// # Style ranges and clicks
//
// A Line also carries a slice of Range entries mapping cell columns
// back to a command the status line binds to that region, so
// "click on the window tab to select-window" works. render stashes
// these; the server loop translates mouse events into command
// dispatches.
//
// # I/O surfaces
//
// None. Render is a pure function of its inputs. Any I/O implied by
// #(...) lives in the format.Env.ShellRunner the caller supplies.
//
// # In isolation
//
// Renderable against a stub format.Context. Golden-file tests verify
// particular format strings produce the right cells without ever
// booting a real server or pane.
//
// # Non-goals
//
// Not drawn here. render.Compose places the Lines. Not evaluated on
// a timer here; the server loop calls Render on its redraw cadence.
package status
