package cmdq

import "github.com/dhamidi/dmux/internal/cmd"

// Item is one queued command invocation: the Command to run, the
// argv to hand Exec, and the Item the command's Exec call will see.
// The cmd.Item is embedded by field rather than interface-inherited
// so callers can distinguish the queue-level Item from the command
// framework's Item type (see doc.go note on Item lifecycle).
type Item struct {
	// Cmd is the command to invoke.
	Cmd cmd.Command
	// Argv is the full argv including argv[0] (the command name).
	Argv []string
	// CmdItem is the cmd.Item handed to Cmd.Exec.
	CmdItem cmd.Item
}

// List is a sequence of pending Items drained by the server's main
// goroutine. The M1 List is a plain slice: no per-client ownership,
// no fan-in for continuations, no park/resume. Commands are executed
// synchronously in append order and removed from the list by Drain.
type List struct {
	items []Item
}

// Append adds it to the tail of the list.
func (l *List) Append(it Item) {
	l.items = append(l.items, it)
}

// Len reports the number of pending items.
func (l *List) Len() int { return len(l.items) }

// Drain runs every pending Item synchronously in append order and
// returns their Results in the same order. After Drain the list is
// empty regardless of whether any Result was an error; collecting
// errors is the caller's job.
//
// TODO(m1:cmdq-await): handle cmd.Await-returning Results here by
// parking the List at the head item and resuming on continuation.
// M1 commands are all synchronous, so every Result from Exec is
// terminal and we walk the list straight through.
func (l *List) Drain() []cmd.Result {
	results := make([]cmd.Result, 0, len(l.items))
	for _, it := range l.items {
		results = append(results, it.Cmd.Exec(it.CmdItem, it.Argv))
	}
	l.items = l.items[:0]
	return results
}
