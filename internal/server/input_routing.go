package server

import (
	"github.com/dhamidi/dmux/internal/keys"
	"github.com/dhamidi/dmux/internal/termin"
)

// paneWriter is the narrow write surface routeInput needs from a pane
// — just enough to forward unbound raw bytes to the pty. Split out
// from *pane.Pane so tests can supply a fake without standing up a
// real pane.
type paneWriter interface {
	Write(p []byte) (int, error)
}

// bindingDispatcher is the narrow callback routeInput uses to run a
// bound command. Returning the helper as an interface keeps pump-side
// concerns (mutable *session.Window / pane.Subscription / logging)
// out of this routing loop — routeInput only knows "the user pressed
// a bound key, please fire the argv". The dispatcher is allowed to
// mutate state the caller cares about (e.g. swap the active pane
// after a window-switch command); routeInput re-checks the table
// after each dispatch so the caller is free to set it too.
type bindingDispatcher interface {
	dispatch(argv []string)
}

// dispatcherFunc adapts a plain func to bindingDispatcher. Useful for
// tests and for the pump's own closure-shaped dispatcher.
type dispatcherFunc func(argv []string)

func (f dispatcherFunc) dispatch(argv []string) { f(argv) }

// routeInput is the per-Input-frame routing loop extracted from pump
// for testability. It walks the parser's emissions; KeyEvents whose
// normalized KeyCode resolves in curTable either swap the pump's
// current table (SwitchTable bindings) or dispatch a command
// (command bindings, after which the pump returns to the root
// table). Unbound keys and non-key events forward their raw bytes
// to the pane's pty.
//
// The returned table is the table the pump should be on after this
// batch of emissions has been processed — curTable unchanged if no
// binding fired, or the swapped/root table otherwise. Callers pass
// it back in as curTable on the next call.
//
// A binding that names a table the caller does not know about is
// treated as a command miss: the pump falls back to the root table
// rather than getting stuck in a dead state. The same applies when
// the root table itself is missing — routeInput gives up and leaves
// curTable unchanged so the caller can log and move on.
func routeInput(
	emissions []termin.Emission,
	curTable *keys.Table,
	rootTable *keys.Table,
	keyTables map[string]*keys.Table,
	dispatch bindingDispatcher,
	pw paneWriter,
) *keys.Table {
	for _, em := range emissions {
		if ke, ok := em.Event.(termin.KeyEvent); ok {
			if code, okc := keys.Code(ke.Event); okc {
				if binding := curTable.Lookup(code); binding != nil {
					if binding.SwitchTable != "" {
						if next, ok := keyTables[binding.SwitchTable]; ok {
							curTable = next
						} else if rootTable != nil {
							curTable = rootTable
						}
						continue
					}
					dispatch.dispatch(binding.Argv)
					if rootTable != nil {
						curTable = rootTable
					}
					continue
				}
			}
		}
		if len(em.Bytes) > 0 {
			_, _ = pw.Write(em.Bytes)
		}
	}
	return curTable
}
